package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

type accountSharePolicyRepository struct {
	db *sql.DB
}

func NewAccountSharePolicyRepository(_ *dbent.Client, sqlDB *sql.DB) service.AccountSharePolicyRepository {
	return &accountSharePolicyRepository{db: sqlDB}
}

func (r *accountSharePolicyRepository) ListAccountSharePolicies(ctx context.Context, params pagination.PaginationParams, filters service.AccountSharePolicyFilters) ([]service.AccountSharePolicy, *pagination.PaginationResult, error) {
	if r == nil || r.db == nil {
		return nil, nil, errors.New("account share policy repository db is nil")
	}
	where, args := accountSharePolicyWhere(filters)
	var total int64
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM account_share_policies WHERE deleted_at IS NULL"+where, args...).Scan(&total); err != nil {
		return nil, nil, err
	}

	limit := params.Limit()
	query := accountSharePolicyTableSelect + " WHERE deleted_at IS NULL" + where +
		" ORDER BY effective_at DESC, id DESC LIMIT $" + strconv.Itoa(len(args)+1) +
		" OFFSET $" + strconv.Itoa(len(args)+2)
	args = append(args, limit, params.Offset())
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	policies := make([]service.AccountSharePolicy, 0, limit)
	for rows.Next() {
		policy, scanErr := scanAccountSharePolicy(rows)
		if scanErr != nil {
			return nil, nil, scanErr
		}
		policies = append(policies, *policy)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	return policies, accountSharePolicyPagination(total, params), nil
}

func (r *accountSharePolicyRepository) GetAccountSharePolicyByID(ctx context.Context, id int64) (*service.AccountSharePolicy, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("account share policy repository db is nil")
	}
	policy, err := scanAccountSharePolicy(r.db.QueryRowContext(ctx, accountSharePolicyTableSelect+" WHERE id = $1 AND deleted_at IS NULL", id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, service.ErrAccountSharePolicyNotFound
	}
	if err != nil {
		return nil, err
	}
	return policy, nil
}

func (r *accountSharePolicyRepository) ResolveEnabledAccountSharePolicy(ctx context.Context, accountID int64, groupID *int64, platform string, explicitPolicyID *int64) (*service.AccountSharePolicy, error) {
	if explicitPolicyID != nil && *explicitPolicyID > 0 {
		policy, found, err := r.queryEnabledAccountSharePolicy(ctx, "id = $1", *explicitPolicyID)
		if err != nil || found {
			return policy, err
		}
	}
	if accountID > 0 {
		if policy, found, err := r.queryEnabledAccountSharePolicy(ctx, "scope_type = 'account' AND scope_id = $1", accountID); err != nil || found {
			return policy, err
		}
	}
	if groupID != nil && *groupID > 0 {
		if policy, found, err := r.queryEnabledAccountSharePolicy(ctx, "scope_type = 'group' AND scope_id = $1", *groupID); err != nil || found {
			return policy, err
		}
	}
	if strings.TrimSpace(platform) != "" {
		if policy, found, err := r.queryEnabledAccountSharePolicy(ctx, "scope_type = 'platform' AND platform = $1", strings.TrimSpace(platform)); err != nil || found {
			return policy, err
		}
	}
	policy, _, err := r.queryEnabledAccountSharePolicy(ctx, "scope_type = 'global'")
	return policy, err
}

func (r *accountSharePolicyRepository) queryEnabledAccountSharePolicy(ctx context.Context, predicate string, args ...any) (*service.AccountSharePolicy, bool, error) {
	policy, err := scanAccountSharePolicy(r.db.QueryRowContext(ctx,
		accountSharePolicyTableSelect+" WHERE deleted_at IS NULL AND enabled = TRUE AND effective_at <= NOW() AND "+predicate+
			" ORDER BY effective_at DESC, version DESC, id DESC LIMIT 1", args...))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return policy, true, nil
}

func (r *accountSharePolicyRepository) CreateAccountSharePolicy(ctx context.Context, input service.CreateAccountSharePolicyInput) (*service.AccountSharePolicy, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("account share policy repository db is nil")
	}
	row := r.db.QueryRowContext(ctx, `WITH policy AS (
		INSERT INTO account_share_policies (
			scope_type, scope_id, platform, owner_share_ratio, invite_share_ratio,
			enabled, effective_at, created_by_admin_id
		) VALUES ($1, $2, $3, $4::numeric, $5::numeric, $6, $7, $8)
		RETURNING id, scope_type, scope_id, platform, owner_share_ratio::text,
			invite_share_ratio::text, version, enabled, effective_at,
			created_by_admin_id, created_at, updated_at, deleted_at
	)
	SELECT id, scope_type, scope_id, platform, owner_share_ratio::text,
		invite_share_ratio::text, version, enabled, effective_at,
		created_by_admin_id, created_at, updated_at, deleted_at
	FROM policy`,
		input.ScopeType,
		nullableSharePolicyInt64(input.ScopeID),
		nullableSharePolicyString(input.Platform),
		strconv.FormatFloat(input.OwnerShareRatio, 'f', 6, 64),
		strconv.FormatFloat(input.InviteShareRatio, 'f', 6, 64),
		valueOrDefaultBool(input.Enabled, true),
		valueOrDefaultTime(input.EffectiveAt),
		nullableSharePolicyInt64(input.CreatedByAdminID),
	)
	return scanAccountSharePolicy(row)
}

func (r *accountSharePolicyRepository) UpdateAccountSharePolicy(ctx context.Context, id int64, input service.UpdateAccountSharePolicyInput) (*service.AccountSharePolicy, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("account share policy repository db is nil")
	}
	current, err := r.GetAccountSharePolicyByID(ctx, id)
	if err != nil {
		return nil, err
	}
	scopeType := current.ScopeType
	scopeID := current.ScopeID
	platform := current.Platform
	ownerRatio := current.OwnerShareRatio
	inviteRatio := current.InviteShareRatio
	enabled := current.Enabled
	effectiveAt := current.EffectiveAt
	if input.ScopeType != nil {
		scopeType = *input.ScopeType
	}
	if input.ScopeID != nil {
		scopeID = input.ScopeID
	}
	if input.Platform != nil {
		platform = input.Platform
	}
	if input.OwnerShareRatio != nil {
		ownerRatio = *input.OwnerShareRatio
	}
	if input.InviteShareRatio != nil {
		inviteRatio = *input.InviteShareRatio
	}
	if input.Enabled != nil {
		enabled = *input.Enabled
	}
	if input.EffectiveAt != nil {
		effectiveAt = *input.EffectiveAt
	}
	row := r.db.QueryRowContext(ctx, `WITH policy AS (
		UPDATE account_share_policies
		SET scope_type = $2, scope_id = $3, platform = $4,
			owner_share_ratio = $5::numeric, invite_share_ratio = $6::numeric,
			enabled = $7, effective_at = $8, version = version + 1, updated_at = NOW()
		WHERE id = $1 AND deleted_at IS NULL
		RETURNING id, scope_type, scope_id, platform, owner_share_ratio::text,
			invite_share_ratio::text, version, enabled, effective_at,
			created_by_admin_id, created_at, updated_at, deleted_at
	)
	SELECT id, scope_type, scope_id, platform, owner_share_ratio::text,
		invite_share_ratio::text, version, enabled, effective_at,
		created_by_admin_id, created_at, updated_at, deleted_at
	FROM policy`,
		id,
		scopeType,
		nullableSharePolicyInt64(scopeID),
		nullableSharePolicyString(platform),
		strconv.FormatFloat(ownerRatio, 'f', 6, 64),
		strconv.FormatFloat(inviteRatio, 'f', 6, 64),
		enabled,
		effectiveAt,
	)
	policy, err := scanAccountSharePolicy(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, service.ErrAccountSharePolicyNotFound
	}
	return policy, err
}

func (r *accountSharePolicyRepository) DeleteAccountSharePolicy(ctx context.Context, id int64) error {
	if r == nil || r.db == nil {
		return errors.New("account share policy repository db is nil")
	}
	result, err := r.db.ExecContext(ctx, `UPDATE account_share_policies SET deleted_at = NOW(), updated_at = NOW() WHERE id = $1 AND deleted_at IS NULL`, id)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return service.ErrAccountSharePolicyNotFound
	}
	return nil
}

const accountSharePolicySelect = `SELECT id, scope_type, scope_id, platform,
	owner_share_ratio::text, invite_share_ratio::text, version, enabled,
	effective_at, created_by_admin_id, created_at, updated_at, deleted_at`

const accountSharePolicyTableSelect = accountSharePolicySelect + ` FROM account_share_policies`

func accountSharePolicyWhere(filters service.AccountSharePolicyFilters) (string, []any) {
	var where strings.Builder
	args := make([]any, 0, 3)
	add := func(condition string, arg any) {
		args = append(args, arg)
		where.WriteString(" AND ")
		where.WriteString(fmt.Sprintf(condition, len(args)))
	}
	if value := strings.TrimSpace(filters.ScopeType); value != "" {
		add("scope_type = $%d", value)
	}
	if value := strings.TrimSpace(filters.Platform); value != "" {
		add("platform = $%d", value)
	}
	if filters.Enabled != nil {
		add("enabled = $%d", *filters.Enabled)
	}
	return where.String(), args
}

type sharePolicyScanner interface{ Scan(...any) error }

func scanAccountSharePolicy(scanner sharePolicyScanner) (*service.AccountSharePolicy, error) {
	var (
		policy         service.AccountSharePolicy
		scopeID        sql.NullInt64
		platform       sql.NullString
		ownerRaw       string
		inviteRaw      string
		createdByAdmin sql.NullInt64
		deletedAt      sql.NullTime
	)
	if err := scanner.Scan(&policy.ID, &policy.ScopeType, &scopeID, &platform, &ownerRaw, &inviteRaw, &policy.Version, &policy.Enabled, &policy.EffectiveAt, &createdByAdmin, &policy.CreatedAt, &policy.UpdatedAt, &deletedAt); err != nil {
		return nil, err
	}
	var err error
	if policy.OwnerShareRatio, err = strconv.ParseFloat(strings.TrimSpace(ownerRaw), 64); err != nil {
		return nil, err
	}
	if policy.InviteShareRatio, err = strconv.ParseFloat(strings.TrimSpace(inviteRaw), 64); err != nil {
		return nil, err
	}
	if scopeID.Valid {
		policy.ScopeID = &scopeID.Int64
	}
	if platform.Valid {
		policy.Platform = &platform.String
	}
	if createdByAdmin.Valid {
		policy.CreatedByAdminID = &createdByAdmin.Int64
	}
	if deletedAt.Valid {
		policy.DeletedAt = &deletedAt.Time
	}
	return &policy, nil
}

func accountSharePolicyPagination(total int64, params pagination.PaginationParams) *pagination.PaginationResult {
	pageSize := params.Limit()
	pages := int64(0)
	if pageSize > 0 && total > 0 {
		pages = (total + int64(pageSize) - 1) / int64(pageSize)
	}
	return &pagination.PaginationResult{Total: total, Page: maxSharePolicyPage(params.Page), PageSize: pageSize, Pages: int(pages)}
}

func maxSharePolicyPage(page int) int {
	if page < 1 {
		return 1
	}
	return page
}

func nullableSharePolicyInt64(value *int64) any {
	if value == nil || *value <= 0 {
		return nil
	}
	return *value
}

func nullableSharePolicyString(value *string) any {
	if value == nil || strings.TrimSpace(*value) == "" {
		return nil
	}
	return strings.TrimSpace(*value)
}

func valueOrDefaultBool(value *bool, fallback bool) bool {
	if value == nil {
		return fallback
	}
	return *value
}

func valueOrDefaultTime(value *time.Time) time.Time {
	if value == nil {
		return time.Now().UTC()
	}
	return *value
}
