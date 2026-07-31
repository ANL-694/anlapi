package service

import (
	"testing"

	"anlapi/internal/config"
	"github.com/stretchr/testify/require"
)

func TestBillingServiceFallbackPricingForKimiK3(t *testing.T) {
	service := NewBillingService(&config.Config{}, nil)

	for _, model := range []string{"kimi-k3", "kimi-k3[1m]", "k3", "k3-256k", "provider/k3"} {
		pricing := service.getFallbackPricing(model)
		require.NotNil(t, pricing, model)
		require.Equal(t, 3e-6, pricing.InputPricePerToken, model)
		require.Equal(t, 15e-6, pricing.OutputPricePerToken, model)
	}

	for _, model := range []string{"kimi-k30", "provider/k3-custom", "k3-custom"} {
		require.NotEqual(t, service.fallbackPrices["kimi-k3"], service.getFallbackPricing(model), model)
	}
}
