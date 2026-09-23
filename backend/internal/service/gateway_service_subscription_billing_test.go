//go:build unit

package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestBuildUsageBillingCommand_SubscriptionAppliesRateMultiplier locks in the fix
// that subscription-mode billing honours the group (and any user-specific) rate
// multiplier — i.e. cmd.SubscriptionCost tracks ActualCost (= TotalCost *
// RateMultiplier), not raw TotalCost.
func TestBuildUsageBillingCommand_SubscriptionAppliesRateMultiplier(t *testing.T) {
	t.Parallel()

	groupID := int64(7)
	subID := int64(42)

	tests := []struct {
		name           string
		totalCost      float64
		actualCost     float64
		isSubscription bool
		wantSub        float64
		wantBalance    float64
	}{
		{
			name:           "subscription with 2x multiplier consumes 2x quota",
			totalCost:      1.0,
			actualCost:     2.0,
			isSubscription: true,
			wantSub:        2.0,
			wantBalance:    0,
		},
		{
			name:           "subscription with 0.5x multiplier consumes 0.5x quota",
			totalCost:      1.0,
			actualCost:     0.5,
			isSubscription: true,
			wantSub:        0.5,
			wantBalance:    0,
		},
		{
			name:           "free subscription (multiplier 0) consumes no quota",
			totalCost:      1.0,
			actualCost:     0,
			isSubscription: true,
			wantSub:        0,
			wantBalance:    0,
		},
		{
			name:           "subscription follows actual cost even when base cost is zero",
			totalCost:      0,
			actualCost:     0.25,
			isSubscription: true,
			wantSub:        0.25,
			wantBalance:    0,
		},
		{
			name:           "balance billing keeps using ActualCost (regression)",
			totalCost:      1.0,
			actualCost:     2.0,
			isSubscription: false,
			wantSub:        0,
			wantBalance:    2.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			p := &postUsageBillingParams{
				Cost:               &CostBreakdown{TotalCost: tt.totalCost, ActualCost: tt.actualCost},
				User:               &User{ID: 1},
				APIKey:             &APIKey{ID: 2, GroupID: &groupID},
				Account:            &Account{ID: 3},
				Subscription:       &UserSubscription{ID: subID},
				IsSubscriptionBill: tt.isSubscription,
			}

			cmd := buildUsageBillingCommand("req-1", nil, p)
			if cmd == nil {
				t.Fatal("buildUsageBillingCommand returned nil")
			}
			if cmd.SubscriptionCost != tt.wantSub {
				t.Errorf("SubscriptionCost = %v, want %v", cmd.SubscriptionCost, tt.wantSub)
			}
			if cmd.BalanceCost != tt.wantBalance {
				t.Errorf("BalanceCost = %v, want %v", cmd.BalanceCost, tt.wantBalance)
			}
		})
	}
}

func TestBuildUsageBillingCommand_SeparatesCustomerAndAccountCostScopes(t *testing.T) {
	t.Parallel()

	groupID := int64(17)
	accountRate := 1.5
	p := &postUsageBillingParams{
		Cost: &CostBreakdown{
			TotalCost:  10,
			ActualCost: 4,
		},
		User: &User{ID: 1},
		APIKey: &APIKey{
			ID:          2,
			GroupID:     &groupID,
			Quota:       100,
			RateLimit5h: 100,
		},
		Account: &Account{
			ID:   3,
			Type: AccountTypeAPIKey,
			Extra: map[string]any{
				"quota_limit": 100.0,
			},
		},
		AccountRateMultiplier: accountRate,
		APIKeyService:         &openAIRecordUsageAPIKeyQuotaStub{},
	}

	cmd := buildUsageBillingCommand("scope-separation", &UsageLog{
		PricingVersion: "pv1-scope",
	}, p)
	require.NotNil(t, cmd)

	// Customer-facing scopes use ActualCost; account quota uses raw upstream
	// cost multiplied by the account-specific rate, and must not reuse the
	// customer charge.
	require.Equal(t, 4.0, cmd.BalanceCost)
	require.Equal(t, 4.0, cmd.APIKeyQuotaCost)
	require.Equal(t, 4.0, cmd.APIKeyRateLimitCost)
	require.Equal(t, 15.0, cmd.AccountQuotaCost)
	require.Equal(t, "pv1-scope", cmd.PricingVersion)
}
