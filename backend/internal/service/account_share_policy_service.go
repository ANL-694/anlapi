package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
)

const (
	AccountSharePolicyScopeGlobal   = "global"
	AccountSharePolicyScopePlatform = "platform"
	AccountSharePolicyScopeGroup    = "group"
	AccountSharePolicyScopeAccount  = "account"
)

var ErrAccountSharePolicyNotFound = infraerrors.NotFound("ACCOUNT_SHARE_POLICY_NOT_FOUND", "account share policy not found")

// AccountSharePolicy is the versioned, request-time policy used by public
// account settlement. Ratios are represented as fractions (0.1 = 10%).
type AccountSharePolicy struct {
	ID               int64      `json:"id"`
	ScopeType        string     `json:"scope_type"`
	ScopeID          *int64     `json:"scope_id,omitempty"`
	Platform         *string    `json:"platform,omitempty"`
	OwnerShareRatio  float64    `json:"owner_share_ratio"`
	InviteShareRatio float64    `json:"invite_share_ratio"`
	Version          int        `json:"version"`
	Enabled          bool       `json:"enabled"`
	EffectiveAt      time.Time  `json:"effective_at"`
	CreatedByAdminID *int64     `json:"created_by_admin_id,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
	DeletedAt        *time.Time `json:"deleted_at,omitempty"`
}

type AccountSharePolicyFilters struct {
	ScopeType string
	Platform  string
	Enabled   *bool
}

type CreateAccountSharePolicyInput struct {
	ScopeType        string
	ScopeID          *int64
	Platform         *string
	OwnerShareRatio  float64
	InviteShareRatio float64
	Enabled          *bool
	EffectiveAt      *time.Time
	CreatedByAdminID *int64
}

type UpdateAccountSharePolicyInput struct {
	ScopeType        *string
	ScopeID          *int64
	Platform         *string
	OwnerShareRatio  *float64
	InviteShareRatio *float64
	Enabled          *bool
	EffectiveAt      *time.Time
}

type AccountSharePolicyRepository interface {
	ListAccountSharePolicies(context.Context, pagination.PaginationParams, AccountSharePolicyFilters) ([]AccountSharePolicy, *pagination.PaginationResult, error)
	GetAccountSharePolicyByID(context.Context, int64) (*AccountSharePolicy, error)
	ResolveEnabledAccountSharePolicy(context.Context, int64, *int64, string, *int64) (*AccountSharePolicy, error)
	CreateAccountSharePolicy(context.Context, CreateAccountSharePolicyInput) (*AccountSharePolicy, error)
	UpdateAccountSharePolicy(context.Context, int64, UpdateAccountSharePolicyInput) (*AccountSharePolicy, error)
	DeleteAccountSharePolicy(context.Context, int64) error
}

type AccountSharePolicyService struct {
	repo AccountSharePolicyRepository
}

func NewAccountSharePolicyService(repo AccountSharePolicyRepository) *AccountSharePolicyService {
	return &AccountSharePolicyService{repo: repo}
}

func (s *AccountSharePolicyService) List(ctx context.Context, params pagination.PaginationParams, filters AccountSharePolicyFilters) ([]AccountSharePolicy, *pagination.PaginationResult, error) {
	if s == nil || s.repo == nil {
		return nil, nil, fmt.Errorf("account share policy repository is not configured")
	}
	return s.repo.ListAccountSharePolicies(ctx, params, filters)
}

func (s *AccountSharePolicyService) GetByID(ctx context.Context, id int64) (*AccountSharePolicy, error) {
	if id <= 0 {
		return nil, ErrAccountSharePolicyNotFound
	}
	return s.repo.GetAccountSharePolicyByID(ctx, id)
}

func (s *AccountSharePolicyService) Create(ctx context.Context, input CreateAccountSharePolicyInput) (*AccountSharePolicy, error) {
	normalized, err := normalizeAccountSharePolicyInput(input)
	if err != nil {
		return nil, err
	}
	return s.repo.CreateAccountSharePolicy(ctx, normalized)
}

func (s *AccountSharePolicyService) Update(ctx context.Context, id int64, input UpdateAccountSharePolicyInput) (*AccountSharePolicy, error) {
	if id <= 0 {
		return nil, ErrAccountSharePolicyNotFound
	}
	existing, err := s.repo.GetAccountSharePolicyByID(ctx, id)
	if err != nil {
		return nil, err
	}
	merged := CreateAccountSharePolicyInput{
		ScopeType:        existing.ScopeType,
		ScopeID:          existing.ScopeID,
		Platform:         existing.Platform,
		OwnerShareRatio:  existing.OwnerShareRatio,
		InviteShareRatio: existing.InviteShareRatio,
		Enabled:          sharePolicyBoolPtr(existing.Enabled),
		EffectiveAt:      sharePolicyTimePtr(existing.EffectiveAt),
	}
	if input.ScopeType != nil {
		merged.ScopeType = *input.ScopeType
	}
	if input.ScopeID != nil {
		if *input.ScopeID <= 0 {
			merged.ScopeID = nil
		} else {
			merged.ScopeID = input.ScopeID
		}
	}
	if input.Platform != nil {
		value := strings.TrimSpace(*input.Platform)
		if value == "" {
			merged.Platform = nil
		} else {
			merged.Platform = &value
		}
	}
	if input.OwnerShareRatio != nil {
		merged.OwnerShareRatio = *input.OwnerShareRatio
	}
	if input.InviteShareRatio != nil {
		merged.InviteShareRatio = *input.InviteShareRatio
	}
	if input.Enabled != nil {
		merged.Enabled = input.Enabled
	}
	if input.EffectiveAt != nil {
		merged.EffectiveAt = input.EffectiveAt
	}
	normalized, err := normalizeAccountSharePolicyInput(merged)
	if err != nil {
		return nil, err
	}
	return s.repo.UpdateAccountSharePolicy(ctx, id, UpdateAccountSharePolicyInput{
		ScopeType:        sharePolicyStringPtr(normalized.ScopeType),
		ScopeID:          normalized.ScopeID,
		Platform:         normalized.Platform,
		OwnerShareRatio:  sharePolicyFloatPtr(normalized.OwnerShareRatio),
		InviteShareRatio: sharePolicyFloatPtr(normalized.InviteShareRatio),
		Enabled:          normalized.Enabled,
		EffectiveAt:      normalized.EffectiveAt,
	})
}

func (s *AccountSharePolicyService) Delete(ctx context.Context, id int64) error {
	if id <= 0 {
		return ErrAccountSharePolicyNotFound
	}
	return s.repo.DeleteAccountSharePolicy(ctx, id)
}

func normalizeAccountSharePolicyInput(input CreateAccountSharePolicyInput) (CreateAccountSharePolicyInput, error) {
	input.ScopeType = normalizeAccountSharePolicyScope(input.ScopeType)
	if input.OwnerShareRatio < 0 || input.OwnerShareRatio > 1 {
		return input, infraerrors.BadRequest("ACCOUNT_SHARE_OWNER_RATIO_INVALID", "owner_share_ratio must be between 0 and 1")
	}
	if input.InviteShareRatio < 0 || input.InviteShareRatio > 1 {
		return input, infraerrors.BadRequest("ACCOUNT_SHARE_INVITE_RATIO_INVALID", "invite_share_ratio must be between 0 and 1")
	}
	if input.OwnerShareRatio+input.InviteShareRatio > 1 {
		return input, infraerrors.BadRequest("ACCOUNT_SHARE_RATIO_SUM_INVALID", "owner_share_ratio plus invite_share_ratio must be less than or equal to 1")
	}
	if input.Enabled == nil {
		input.Enabled = sharePolicyBoolPtr(true)
	}
	if input.EffectiveAt == nil {
		now := time.Now().UTC()
		input.EffectiveAt = &now
	}
	if input.ScopeID != nil && *input.ScopeID <= 0 {
		input.ScopeID = nil
	}
	if input.Platform != nil {
		platform := strings.TrimSpace(*input.Platform)
		if platform == "" {
			input.Platform = nil
		} else {
			input.Platform = &platform
		}
	}
	switch input.ScopeType {
	case AccountSharePolicyScopeGlobal:
		input.ScopeID = nil
		input.Platform = nil
	case AccountSharePolicyScopePlatform:
		if input.Platform == nil {
			return input, infraerrors.BadRequest("ACCOUNT_SHARE_PLATFORM_REQUIRED", "platform is required for a platform policy")
		}
		input.ScopeID = nil
	case AccountSharePolicyScopeGroup, AccountSharePolicyScopeAccount:
		if input.ScopeID == nil {
			return input, infraerrors.BadRequest("ACCOUNT_SHARE_SCOPE_ID_REQUIRED", "scope_id is required for this policy scope")
		}
		input.Platform = nil
	default:
		return input, infraerrors.BadRequest("ACCOUNT_SHARE_SCOPE_INVALID", "scope_type must be global, platform, group, or account")
	}
	return input, nil
}

func normalizeAccountSharePolicyScope(scope string) string {
	switch strings.ToLower(strings.TrimSpace(scope)) {
	case "", AccountSharePolicyScopeGlobal:
		return AccountSharePolicyScopeGlobal
	case AccountSharePolicyScopePlatform, AccountSharePolicyScopeGroup, AccountSharePolicyScopeAccount:
		return strings.ToLower(strings.TrimSpace(scope))
	default:
		return strings.ToLower(strings.TrimSpace(scope))
	}
}

func sharePolicyBoolPtr(value bool) *bool           { return &value }
func sharePolicyFloatPtr(value float64) *float64    { return &value }
func sharePolicyStringPtr(value string) *string     { return &value }
func sharePolicyTimePtr(value time.Time) *time.Time { return &value }
