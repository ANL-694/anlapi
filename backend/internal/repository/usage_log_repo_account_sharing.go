package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

// GetAccountSharingDashboard returns owner-scoped self usage and settlement
// data. Private accounts contribute to the summary but are never returned in
// the account detail list.
func (r *usageLogRepository) GetAccountSharingDashboard(ctx context.Context, q service.AccountSharingQuery) (*service.AccountSharingDashboard, error) {
	result := &service.AccountSharingDashboard{
		Accounts:    make([]service.AccountSharingAccount, 0),
		Trend:       make([]service.AccountSharingTrendPoint, 0),
		StartDate:   q.StartTime.Format("2006-01-02"),
		EndDate:     q.EndTime.Add(-time.Nanosecond).Format("2006-01-02"),
		Granularity: q.Granularity,
	}

	const accountQuery = `
		WITH self_usage AS (
			SELECT
				ul.account_id,
				COUNT(*)::bigint AS self_requests,
				COALESCE(SUM(ul.input_tokens + ul.output_tokens + ul.cache_creation_tokens + ul.cache_read_tokens), 0)::bigint AS self_tokens,
				COALESCE(SUM(ul.actual_cost), 0) AS self_actual_cost,
				COALESCE(SUM(COALESCE(ul.account_stats_cost, ul.total_cost) * COALESCE(ul.account_rate_multiplier, 1)), 0) AS self_account_cost
			FROM usage_logs ul
			JOIN accounts a ON a.id = ul.account_id
			WHERE a.owner_user_id = $1
			  AND a.deleted_at IS NULL
			  AND ul.user_id = $1
			  AND ul.created_at >= $2
			  AND ul.created_at < $3
			  AND ` + usageLogSuccessFilterUL + `
			GROUP BY ul.account_id
		),
		external_usage AS (
			SELECT
				account_id,
				COUNT(*)::bigint AS external_requests,
				COALESCE(SUM(consumer_charge), 0) AS external_consumer_charge,
				COALESCE(SUM(account_cost), 0) AS external_account_cost,
				COALESCE(SUM(owner_credit), 0) AS external_owner_credit,
				COALESCE(SUM(platform_fee), 0) AS external_platform_fee
			FROM account_share_settlement_entries ase
			JOIN accounts a
			  ON a.id = ase.account_id
			 AND a.owner_user_id = ase.owner_user_id
			 AND a.deleted_at IS NULL
			WHERE ase.owner_user_id = $1
			  AND ase.consumer_user_id <> ase.owner_user_id
			  AND ase.status = 'applied'
			  AND ase.created_at >= $2
			  AND ase.created_at < $3
			GROUP BY ase.account_id
		),
		account_stats AS (
			SELECT
				a.id AS account_id,
				a.name,
				a.platform,
				a.share_mode,
				a.share_status,
				COALESCE(s.self_requests, 0)::bigint AS self_requests,
				COALESCE(s.self_tokens, 0)::bigint AS self_tokens,
				COALESCE(s.self_actual_cost, 0) AS self_actual_cost,
				COALESCE(s.self_account_cost, 0) AS self_account_cost,
				COALESCE(e.external_requests, 0)::bigint AS external_requests,
				COALESCE(e.external_consumer_charge, 0) AS external_consumer_charge,
				COALESCE(e.external_account_cost, 0) AS external_account_cost,
				COALESCE(e.external_owner_credit, 0) AS external_owner_credit,
				COALESCE(e.external_platform_fee, 0) AS external_platform_fee,
				(COALESCE(s.self_account_cost, 0) + COALESCE(e.external_account_cost, 0)) AS sort_account_cost,
				a.created_at
			FROM accounts a
			LEFT JOIN self_usage s ON s.account_id = a.id
			LEFT JOIN external_usage e ON e.account_id = a.id
			WHERE a.owner_user_id = $1
			  AND a.deleted_at IS NULL
		),
		summary AS (
			SELECT
				COUNT(*)::bigint AS owned_accounts,
				COUNT(*) FILTER (WHERE share_mode = 'public')::bigint AS public_accounts,
				COUNT(*) FILTER (WHERE share_mode = 'private')::bigint AS private_accounts,
				COUNT(*) FILTER (WHERE share_mode = 'public' AND share_status = 'pending')::bigint AS public_pending_accounts,
				COUNT(*) FILTER (WHERE share_mode = 'public' AND share_status = 'approved')::bigint AS public_approved_accounts,
				COUNT(*) FILTER (WHERE share_mode = 'public' AND share_status = 'suspended')::bigint AS public_suspended_accounts,
				COALESCE(SUM(self_requests), 0)::bigint AS self_requests,
				COALESCE(SUM(self_tokens), 0)::bigint AS self_tokens,
				COALESCE(SUM(self_actual_cost), 0) AS self_actual_cost,
				COALESCE(SUM(self_account_cost), 0) AS self_account_cost,
				COALESCE(SUM(external_requests), 0)::bigint AS external_requests,
				COALESCE(SUM(external_consumer_charge), 0) AS external_consumer_charge,
				COALESCE(SUM(external_account_cost), 0) AS external_account_cost,
				COALESCE(SUM(external_owner_credit), 0) AS external_owner_credit,
				COALESCE(SUM(external_platform_fee), 0) AS external_platform_fee
			FROM account_stats
		),
		paged_accounts AS (
			SELECT *
			FROM account_stats
			WHERE share_mode = 'public'
			ORDER BY sort_account_cost DESC, created_at DESC, account_id DESC
			LIMIT $4 OFFSET $5
		)
		SELECT
			s.owned_accounts,
			s.public_accounts,
			s.private_accounts,
			s.public_pending_accounts,
			s.public_approved_accounts,
			s.public_suspended_accounts,
			s.self_requests,
			s.self_tokens,
			s.self_actual_cost,
			s.self_account_cost,
			s.external_requests,
			s.external_consumer_charge,
			s.external_account_cost,
			s.external_owner_credit,
			s.external_platform_fee,
			p.account_id,
			p.name,
			p.platform,
			p.share_mode,
			p.share_status,
			p.self_requests,
			p.self_tokens,
			p.self_actual_cost,
			p.self_account_cost,
			p.external_requests,
			p.external_consumer_charge,
			p.external_account_cost,
			p.external_owner_credit,
			p.external_platform_fee
		FROM summary s
		LEFT JOIN paged_accounts p ON TRUE
		ORDER BY p.sort_account_cost DESC NULLS LAST, p.created_at DESC NULLS LAST, p.account_id DESC NULLS LAST
	`

	rows, err := r.sql.QueryContext(ctx, accountQuery, q.UserID, q.StartTime, q.EndTime, q.PageSize, (q.Page-1)*q.PageSize)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var summary service.AccountSharingSummary
	for rows.Next() {
		var (
			accountID              sql.NullInt64
			name                   sql.NullString
			platform               sql.NullString
			shareMode              sql.NullString
			shareStatus            sql.NullString
			selfRequests           sql.NullInt64
			selfTokens             sql.NullInt64
			selfActualCost         sql.NullFloat64
			selfAccountCost        sql.NullFloat64
			externalRequests       sql.NullInt64
			externalConsumerCharge sql.NullFloat64
			externalAccountCost    sql.NullFloat64
			externalOwnerCredit    sql.NullFloat64
			externalPlatformFee    sql.NullFloat64
		)
		if err := rows.Scan(
			&summary.OwnedAccounts,
			&summary.PublicAccounts,
			&summary.PrivateAccounts,
			&summary.PublicPendingAccounts,
			&summary.PublicApprovedAccounts,
			&summary.PublicSuspendedAccounts,
			&summary.SelfRequests,
			&summary.SelfTokens,
			&summary.SelfActualCost,
			&summary.SelfAccountCost,
			&summary.ExternalRequests,
			&summary.ExternalConsumerCharge,
			&summary.ExternalAccountCost,
			&summary.ExternalOwnerCredit,
			&summary.ExternalPlatformFee,
			&accountID,
			&name,
			&platform,
			&shareMode,
			&shareStatus,
			&selfRequests,
			&selfTokens,
			&selfActualCost,
			&selfAccountCost,
			&externalRequests,
			&externalConsumerCharge,
			&externalAccountCost,
			&externalOwnerCredit,
			&externalPlatformFee,
		); err != nil {
			return nil, err
		}
		if !accountID.Valid {
			continue
		}
		result.Accounts = append(result.Accounts, service.AccountSharingAccount{
			AccountID:              accountID.Int64,
			Name:                   name.String,
			Platform:               platform.String,
			ShareMode:              shareMode.String,
			ShareStatus:            shareStatus.String,
			SelfRequests:           selfRequests.Int64,
			SelfTokens:             selfTokens.Int64,
			SelfActualCost:         selfActualCost.Float64,
			SelfAccountCost:        selfAccountCost.Float64,
			ExternalRequests:       externalRequests.Int64,
			ExternalConsumerCharge: externalConsumerCharge.Float64,
			ExternalAccountCost:    externalAccountCost.Float64,
			ExternalOwnerCredit:    externalOwnerCredit.Float64,
			ExternalPlatformFee:    externalPlatformFee.Float64,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	result.Summary = summary
	result.Summary.TotalAccountCost = summary.SelfAccountCost + summary.ExternalAccountCost
	result.Summary.BalanceNetChange = summary.ExternalOwnerCredit - summary.SelfActualCost
	publicAccountTotal := summary.PublicAccounts
	result.AccountsPagination = service.AccountSharingPagination{
		Total:    publicAccountTotal,
		Page:     q.Page,
		PageSize: q.PageSize,
		Pages:    accountSharingPages(publicAccountTotal, q.PageSize),
	}

	dateFormat := safeDateFormat(q.Granularity)
	trendQuery := fmt.Sprintf(`
		WITH self_usage AS (
			SELECT
				TO_CHAR(ul.created_at, '%s') AS date,
				COUNT(*)::bigint AS self_requests,
				COALESCE(SUM(ul.input_tokens + ul.output_tokens + ul.cache_creation_tokens + ul.cache_read_tokens), 0)::bigint AS self_tokens,
				COALESCE(SUM(ul.actual_cost), 0) AS self_actual_cost,
				COALESCE(SUM(COALESCE(ul.account_stats_cost, ul.total_cost) * COALESCE(ul.account_rate_multiplier, 1)), 0) AS self_account_cost
			FROM usage_logs ul
			JOIN accounts a ON a.id = ul.account_id
			WHERE a.owner_user_id = $1
			  AND a.deleted_at IS NULL
			  AND ul.user_id = $1
			  AND ul.created_at >= $2
			  AND ul.created_at < $3
			  AND %s
			GROUP BY date
		),
		external_usage AS (
			SELECT
				TO_CHAR(ase.created_at, '%s') AS date,
				COUNT(*)::bigint AS external_requests,
				COALESCE(SUM(consumer_charge), 0) AS external_consumer_charge,
				COALESCE(SUM(account_cost), 0) AS external_account_cost,
				COALESCE(SUM(owner_credit), 0) AS external_owner_credit,
				COALESCE(SUM(platform_fee), 0) AS external_platform_fee
			FROM account_share_settlement_entries ase
			JOIN accounts a
			  ON a.id = ase.account_id
			 AND a.owner_user_id = ase.owner_user_id
			 AND a.deleted_at IS NULL
			WHERE ase.owner_user_id = $1
			  AND ase.consumer_user_id <> ase.owner_user_id
			  AND ase.status = 'applied'
			  AND ase.created_at >= $2
			  AND ase.created_at < $3
			GROUP BY date
		)
		SELECT
			COALESCE(s.date, e.date) AS date,
			COALESCE(s.self_requests, 0),
			COALESCE(s.self_tokens, 0),
			COALESCE(s.self_actual_cost, 0),
			COALESCE(s.self_account_cost, 0),
			COALESCE(e.external_requests, 0),
			COALESCE(e.external_consumer_charge, 0),
			COALESCE(e.external_account_cost, 0),
			COALESCE(e.external_owner_credit, 0),
			COALESCE(e.external_platform_fee, 0)
		FROM self_usage s
		FULL OUTER JOIN external_usage e ON e.date = s.date
		ORDER BY date ASC
	`, dateFormat, usageLogSuccessFilterUL, dateFormat)

	trendRows, err := r.sql.QueryContext(ctx, trendQuery, q.UserID, q.StartTime, q.EndTime)
	if err != nil {
		return nil, err
	}
	defer func() { _ = trendRows.Close() }()
	for trendRows.Next() {
		var point service.AccountSharingTrendPoint
		if err := trendRows.Scan(
			&point.Date,
			&point.SelfRequests,
			&point.SelfTokens,
			&point.SelfActualCost,
			&point.SelfAccountCost,
			&point.ExternalRequests,
			&point.ExternalConsumerCharge,
			&point.ExternalAccountCost,
			&point.ExternalOwnerCredit,
			&point.ExternalPlatformFee,
		); err != nil {
			return nil, err
		}
		result.Trend = append(result.Trend, point)
	}
	if err := trendRows.Err(); err != nil {
		return nil, err
	}

	return result, nil
}

func accountSharingPages(total int64, pageSize int) int {
	if pageSize < 1 {
		pageSize = 20
	}
	if total <= 0 {
		return 1
	}
	return int((total + int64(pageSize) - 1) / int64(pageSize))
}
