package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

type AccountShareSettlementQuery struct {
	StartTime time.Time
	EndTime   time.Time
	Page      int
	PageSize  int
	Search    string
	Status    string
}

type AccountShareSettlementItem struct {
	ID                    int64      `json:"id"`
	UsageLogID            *int64     `json:"usage_log_id,omitempty"`
	RequestID             string     `json:"request_id"`
	APIKeyID              int64      `json:"api_key_id"`
	ConsumerUserID        int64      `json:"consumer_user_id"`
	ConsumerEmail         string     `json:"consumer_email"`
	OwnerUserID           int64      `json:"owner_user_id"`
	OwnerEmail            string     `json:"owner_email"`
	InviterUserID         *int64     `json:"inviter_user_id,omitempty"`
	InviterEmail          *string    `json:"inviter_email,omitempty"`
	AccountID             int64      `json:"account_id"`
	AccountName           string     `json:"account_name"`
	Platform              string     `json:"platform"`
	GroupID               *int64     `json:"group_id,omitempty"`
	GroupName             *string    `json:"group_name,omitempty"`
	Model                 *string    `json:"model,omitempty"`
	PolicyID              *int64     `json:"policy_id,omitempty"`
	PolicyVersion         int        `json:"policy_version"`
	ShareModeSnapshot     string     `json:"share_mode_snapshot"`
	ShareStatusSnapshot   string     `json:"share_status_snapshot"`
	ConsumerCharge        float64    `json:"consumer_charge"`
	AccountCost           float64    `json:"account_cost"`
	OwnerShareRatio       float64    `json:"owner_share_ratio"`
	OwnerCredit           float64    `json:"owner_credit"`
	InviteShareRatio      float64    `json:"invite_share_ratio"`
	InviteCredit          float64    `json:"invite_credit"`
	PlatformShareRatio    float64    `json:"platform_share_ratio"`
	PlatformFee           float64    `json:"platform_fee"`
	InviteBoundAtSnapshot *time.Time `json:"invite_bound_at_snapshot,omitempty"`
	InviteExpiresSnapshot *time.Time `json:"invite_expires_at_snapshot,omitempty"`
	Status                string     `json:"status"`
	CreatedAt             time.Time  `json:"created_at"`
}

type AccountShareSettlementService struct {
	db *sql.DB
}

func NewAccountShareSettlementService(db *sql.DB) *AccountShareSettlementService {
	return &AccountShareSettlementService{db: db}
}

func (s *AccountShareSettlementService) List(ctx context.Context, q AccountShareSettlementQuery) ([]AccountShareSettlementItem, int64, error) {
	if s == nil || s.db == nil {
		return nil, 0, errors.New("account share settlement service db is nil")
	}
	if !q.StartTime.Before(q.EndTime) {
		return nil, 0, fmt.Errorf("invalid account share settlement time range")
	}
	page := q.Page
	if page < 1 {
		page = 1
	}
	pageSize := q.PageSize
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	where, args := accountShareSettlementWhere(q)
	var total int64
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM account_share_settlement_entries ase "+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	query := accountShareSettlementSelect + " " + where +
		" ORDER BY ase.created_at DESC, ase.id DESC LIMIT $" + fmt.Sprint(len(args)+1) + " OFFSET $" + fmt.Sprint(len(args)+2)
	args = append(args, pageSize, (page-1)*pageSize)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items := make([]AccountShareSettlementItem, 0, pageSize)
	for rows.Next() {
		item, scanErr := scanAccountShareSettlement(rows)
		if scanErr != nil {
			return nil, 0, scanErr
		}
		items = append(items, *item)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

const accountShareSettlementSelect = `SELECT
	ase.id, ase.usage_log_id, ase.request_id, ase.api_key_id,
	ase.consumer_user_id, cu.email, ase.owner_user_id, ou.email,
	ase.inviter_user_id, iu.email, ase.account_id, a.name, a.platform,
	ase.group_id, g.name, ul.model, ase.policy_id, ase.policy_version,
	ase.share_mode_snapshot, ase.share_status_snapshot,
	ase.consumer_charge::double precision, ase.account_cost::double precision,
	ase.owner_share_ratio::double precision, ase.owner_credit::double precision,
	ase.invite_share_ratio::double precision, ase.invite_credit::double precision,
	ase.platform_share_ratio::double precision, ase.platform_fee::double precision,
	ase.invite_bound_at_snapshot, ase.invite_expires_at_snapshot,
	ase.status, ase.created_at
FROM account_share_settlement_entries ase
JOIN users cu ON cu.id = ase.consumer_user_id
JOIN users ou ON ou.id = ase.owner_user_id
LEFT JOIN users iu ON iu.id = ase.inviter_user_id
JOIN accounts a ON a.id = ase.account_id
LEFT JOIN groups g ON g.id = ase.group_id
LEFT JOIN usage_logs ul ON ul.id = ase.usage_log_id`

func accountShareSettlementWhere(q AccountShareSettlementQuery) (string, []any) {
	conditions := []string{"ase.created_at >= $1", "ase.created_at < $2"}
	args := []any{q.StartTime, q.EndTime}
	if status := strings.TrimSpace(q.Status); status != "" {
		conditions = append(conditions, fmt.Sprintf("ase.status = $%d", len(args)+1))
		args = append(args, status)
	}
	if search := strings.TrimSpace(q.Search); search != "" {
		conditions = append(conditions, fmt.Sprintf(`(
			ase.request_id ILIKE $%d OR cu.email ILIKE $%d OR ou.email ILIKE $%d OR
			COALESCE(iu.email, '') ILIKE $%d OR a.name ILIKE $%d OR a.platform ILIKE $%d
		)`, len(args)+1, len(args)+1, len(args)+1, len(args)+1, len(args)+1, len(args)+1))
		args = append(args, "%"+search+"%")
	}
	return "WHERE " + strings.Join(conditions, " AND "), args
}

type accountShareSettlementScanner interface{ Scan(...any) error }

func scanAccountShareSettlement(scanner accountShareSettlementScanner) (*AccountShareSettlementItem, error) {
	var (
		item            AccountShareSettlementItem
		usageLogID      sql.NullInt64
		inviterUserID   sql.NullInt64
		inviterEmail    sql.NullString
		groupID         sql.NullInt64
		groupName       sql.NullString
		model           sql.NullString
		policyID        sql.NullInt64
		inviteBoundAt   sql.NullTime
		inviteExpiresAt sql.NullTime
	)
	if err := scanner.Scan(
		&item.ID, &usageLogID, &item.RequestID, &item.APIKeyID,
		&item.ConsumerUserID, &item.ConsumerEmail, &item.OwnerUserID, &item.OwnerEmail,
		&inviterUserID, &inviterEmail, &item.AccountID, &item.AccountName, &item.Platform,
		&groupID, &groupName, &model, &policyID, &item.PolicyVersion,
		&item.ShareModeSnapshot, &item.ShareStatusSnapshot,
		&item.ConsumerCharge, &item.AccountCost, &item.OwnerShareRatio, &item.OwnerCredit,
		&item.InviteShareRatio, &item.InviteCredit, &item.PlatformShareRatio, &item.PlatformFee,
		&inviteBoundAt, &inviteExpiresAt, &item.Status, &item.CreatedAt,
	); err != nil {
		return nil, err
	}
	if usageLogID.Valid {
		item.UsageLogID = &usageLogID.Int64
	}
	if inviterUserID.Valid {
		item.InviterUserID = &inviterUserID.Int64
	}
	if inviterEmail.Valid {
		item.InviterEmail = &inviterEmail.String
	}
	if groupID.Valid {
		item.GroupID = &groupID.Int64
	}
	if groupName.Valid {
		item.GroupName = &groupName.String
	}
	if model.Valid {
		item.Model = &model.String
	}
	if policyID.Valid {
		item.PolicyID = &policyID.Int64
	}
	if inviteBoundAt.Valid {
		item.InviteBoundAtSnapshot = &inviteBoundAt.Time
	}
	if inviteExpiresAt.Valid {
		item.InviteExpiresSnapshot = &inviteExpiresAt.Time
	}
	return &item, nil
}
