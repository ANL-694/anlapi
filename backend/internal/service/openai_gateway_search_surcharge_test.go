//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestCalculateOpenAIRecordUsageCost_SearchIsAdditiveToTokens(t *testing.T) {
	t.Parallel()

	price := 10.0 // $10 / 1k searches → 100 searches = $1.0
	svc := &OpenAIGatewayService{
		billingService: newTestBillingService(),
	}
	apiKey := &APIKey{
		Group: &Group{
			SearchPricePer1k: &price,
		},
	}

	// claude-sonnet-4 fallback: Input $3/MTok, Output $15/MTok
	// 1000 in + 500 out → 0.003 + 0.0075 = 0.0105
	// + 100 searches → +1.0 → total 1.0105
	cost, err := svc.calculateOpenAIRecordUsageCost(
		context.Background(),
		&OpenAIForwardResult{SearchCount: 100},
		apiKey,
		[]string{"claude-sonnet-4"},
		1.0,
		1.0,
		1.0,
		1.0,
		UsageTokens{InputTokens: 1000, OutputTokens: 500},
		"",
		boolPtr(false),
		time.Time{},
	)
	require.NoError(t, err)
	require.NotNil(t, cost)
	require.InDelta(t, 1.0105, cost.ActualCost, 1e-9)
	require.InDelta(t, 1.0105, cost.TotalCost, 1e-9)
	tokenOnly, tokenErr := svc.calculateOpenAIRecordUsageCost(
		context.Background(),
		&OpenAIForwardResult{},
		apiKey,
		[]string{"claude-sonnet-4"},
		1.0,
		1.0,
		1.0,
		1.0,
		UsageTokens{InputTokens: 1000, OutputTokens: 500},
		"",
		boolPtr(false),
		time.Time{},
	)
	require.NoError(t, tokenErr)
	searchCost := svc.billingService.CalculateSearchCost(100, &price, 1.0)
	require.Equal(t, CombinePricingSnapshotVersions(tokenOnly.PricingVersion, searchCost.PricingVersion), cost.PricingVersion)
}

func TestCalculateOpenAIRecordUsageCost_SearchOnlyWhenNoTokenPricing(t *testing.T) {
	t.Parallel()

	price := 10.0
	svc := &OpenAIGatewayService{
		billingService: newTestBillingService(),
	}
	apiKey := &APIKey{
		Group: &Group{SearchPricePer1k: &price},
	}
	// Empty model list: token path fails; search-only surcharge still bills.
	cost, err := svc.calculateOpenAIRecordUsageCost(
		context.Background(),
		&OpenAIForwardResult{SearchCount: 100},
		apiKey,
		nil,
		1.0,
		1.0,
		1.0,
		1.0,
		UsageTokens{},
		"",
		boolPtr(false),
		time.Time{},
	)
	require.NoError(t, err)
	require.NotNil(t, cost)
	require.InDelta(t, 1.0, cost.ActualCost, 1e-9)
	require.NotEmpty(t, cost.PricingVersion)
}

func TestCalculateOpenAIRecordUsageCost_FreeSearchStillCombinesPricingVersion(t *testing.T) {
	t.Parallel()

	free := 0.0
	svc := &OpenAIGatewayService{billingService: newTestBillingService()}
	apiKey := &APIKey{Group: &Group{SearchPricePer1k: &free}}
	result := &OpenAIForwardResult{SearchCount: 100}
	tokens := UsageTokens{InputTokens: 1000, OutputTokens: 500}

	cost, err := svc.calculateOpenAIRecordUsageCost(context.Background(), result, apiKey, []string{"claude-sonnet-4"}, 1, 1, 1, 1, tokens, "", boolPtr(false), time.Time{})
	require.NoError(t, err)
	require.NotNil(t, cost)

	tokenOnly, err := svc.calculateOpenAIRecordUsageCost(context.Background(), &OpenAIForwardResult{}, apiKey, []string{"claude-sonnet-4"}, 1, 1, 1, 1, tokens, "", boolPtr(false), time.Time{})
	require.NoError(t, err)
	searchCost := svc.billingService.CalculateSearchCost(100, &free, 1)
	require.Equal(t, tokenOnly.TotalCost, cost.TotalCost)
	require.Equal(t, tokenOnly.ActualCost, cost.ActualCost)
	require.Equal(t, CombinePricingSnapshotVersions(tokenOnly.PricingVersion, searchCost.PricingVersion), cost.PricingVersion)
}

func TestGroupMediaPricingLooksIncomplete_VideoModelPricesComplete(t *testing.T) {
	t.Parallel()
	require.True(t, groupMediaPricingLooksIncomplete(nil))
	require.True(t, groupMediaPricingLooksIncomplete(&Group{}))
	require.False(t, groupMediaPricingLooksIncomplete(&Group{
		VideoModelPrices: map[string]map[string]float64{
			"grok-imagine-video": {"720p": 0.1},
		},
	}))
	price := 10.0
	require.False(t, groupMediaPricingLooksIncomplete(&Group{SearchPricePer1k: &price}))
	require.False(t, groupMediaPricingLooksIncomplete(&Group{AudioRealtimePricePerMin: &price}))
	// Legacy video price alone still marks complete (existing path).
	require.False(t, groupMediaPricingLooksIncomplete(&Group{VideoPrice720P: &price}))
}

func TestCalculateOpenAIRecordUsageCost_TokenPricingErrorNotSwallowedBySearch(t *testing.T) {
	t.Parallel()

	price := 10.0
	svc := &OpenAIGatewayService{
		billingService: newTestBillingService(),
	}
	apiKey := &APIKey{
		Group: &Group{SearchPricePer1k: &price},
	}
	// Unknown model → token pricing fails; search must not replace that with $0/$search bill.
	cost, err := svc.calculateOpenAIRecordUsageCost(
		context.Background(),
		&OpenAIForwardResult{SearchCount: 100},
		apiKey,
		[]string{"totally-unknown-model-xyz-no-pricing"},
		1.0,
		1.0,
		1.0,
		1.0,
		UsageTokens{InputTokens: 1000, OutputTokens: 500},
		"",
		boolPtr(false),
		time.Time{},
	)
	require.Error(t, err)
	require.Nil(t, cost)
}
