package service

import (
	"context"
	"fmt"
	"time"
)

// AccountSharingQuery describes the user-scoped account sharing dashboard.
// The query deliberately uses the existing accounts.owner_user_id contract;
// no external Hub/user/settlement data is mixed into anlapi.
type AccountSharingQuery struct {
	UserID      int64
	StartTime   time.Time
	EndTime     time.Time
	Granularity string
	Page        int
	PageSize    int
}

type AccountSharingSummary struct {
	OwnedAccounts           int64   `json:"owned_accounts"`
	PublicAccounts          int64   `json:"public_accounts"`
	PrivateAccounts         int64   `json:"private_accounts"`
	PublicPendingAccounts   int64   `json:"public_pending_accounts"`
	PublicApprovedAccounts  int64   `json:"public_approved_accounts"`
	PublicSuspendedAccounts int64   `json:"public_suspended_accounts"`
	SelfRequests            int64   `json:"self_requests"`
	SelfTokens              int64   `json:"self_tokens"`
	SelfActualCost          float64 `json:"self_actual_cost"`
	SelfAccountCost         float64 `json:"self_account_cost"`
	ExternalRequests        int64   `json:"external_requests"`
	ExternalConsumerCharge  float64 `json:"external_consumer_charge"`
	ExternalAccountCost     float64 `json:"external_account_cost"`
	ExternalOwnerCredit     float64 `json:"external_owner_credit"`
	ExternalPlatformFee     float64 `json:"external_platform_fee"`
	TotalAccountCost        float64 `json:"total_account_cost"`
	BalanceNetChange        float64 `json:"balance_net_change"`
}

type AccountSharingAccount struct {
	AccountID              int64   `json:"account_id"`
	Name                   string  `json:"name"`
	Platform               string  `json:"platform"`
	ShareMode              string  `json:"share_mode"`
	ShareStatus            string  `json:"share_status"`
	SelfRequests           int64   `json:"self_requests"`
	SelfTokens             int64   `json:"self_tokens"`
	SelfActualCost         float64 `json:"self_actual_cost"`
	SelfAccountCost        float64 `json:"self_account_cost"`
	ExternalRequests       int64   `json:"external_requests"`
	ExternalConsumerCharge float64 `json:"external_consumer_charge"`
	ExternalAccountCost    float64 `json:"external_account_cost"`
	ExternalOwnerCredit    float64 `json:"external_owner_credit"`
	ExternalPlatformFee    float64 `json:"external_platform_fee"`
}

type AccountSharingPagination struct {
	Total    int64 `json:"total"`
	Page     int   `json:"page"`
	PageSize int   `json:"page_size"`
	Pages    int   `json:"pages"`
}

type AccountSharingTrendPoint struct {
	Date                   string  `json:"date"`
	SelfRequests           int64   `json:"self_requests"`
	SelfTokens             int64   `json:"self_tokens"`
	SelfActualCost         float64 `json:"self_actual_cost"`
	SelfAccountCost        float64 `json:"self_account_cost"`
	ExternalRequests       int64   `json:"external_requests"`
	ExternalConsumerCharge float64 `json:"external_consumer_charge"`
	ExternalAccountCost    float64 `json:"external_account_cost"`
	ExternalOwnerCredit    float64 `json:"external_owner_credit"`
	ExternalPlatformFee    float64 `json:"external_platform_fee"`
}

type AccountSharingDashboard struct {
	Summary            AccountSharingSummary      `json:"summary"`
	Accounts           []AccountSharingAccount    `json:"accounts"`
	AccountsPagination AccountSharingPagination   `json:"accounts_pagination"`
	Trend              []AccountSharingTrendPoint `json:"trend"`
	StartDate          string                     `json:"start_date"`
	EndDate            string                     `json:"end_date"`
	Granularity        string                     `json:"granularity"`
}

type accountSharingRepository interface {
	GetAccountSharingDashboard(context.Context, AccountSharingQuery) (*AccountSharingDashboard, error)
}

// GetAccountSharingDashboard returns only usage associated with the current
// user. Repositories that do not implement the contract fail closed instead
// of returning fabricated dashboard data.
func (s *UsageService) GetAccountSharingDashboard(ctx context.Context, query AccountSharingQuery) (*AccountSharingDashboard, error) {
	repo, ok := s.usageRepo.(accountSharingRepository)
	if !ok {
		return nil, fmt.Errorf("account sharing dashboard is not available")
	}
	if query.UserID <= 0 {
		return nil, fmt.Errorf("invalid account sharing user")
	}
	if !query.StartTime.Before(query.EndTime) {
		return nil, fmt.Errorf("invalid account sharing time range")
	}
	if query.Page < 1 || query.PageSize < 1 || query.PageSize > 1000 {
		return nil, fmt.Errorf("invalid account sharing pagination")
	}
	switch query.Granularity {
	case "hour", "day", "week", "month":
	default:
		return nil, fmt.Errorf("invalid account sharing granularity")
	}
	return repo.GetAccountSharingDashboard(ctx, query)
}
