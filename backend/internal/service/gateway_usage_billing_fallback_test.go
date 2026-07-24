//go:build unit

package service

import (
	"context"
	"testing"

	"anlapi/internal/config"

	"github.com/stretchr/testify/require"
)

func TestCompositeBillableModel(t *testing.T) {
	service := &GatewayService{billingService: NewBillingService(&config.Config{}, nil)}
	apiKey := &APIKey{}
	ctx := context.Background()

	require.Equal(t, "claude-opus-4-7", service.compositeBillableModel(ctx, apiKey, "all/claude", "claude-opus-4-7"))
	require.Equal(t, "claude-sonnet-4", service.compositeBillableModel(ctx, apiKey, "team/best", "claude-sonnet-4"))
	require.Equal(t, "claude-sonnet-4", service.compositeBillableModel(ctx, apiKey, "claude-sonnet-4", "claude-sonnet-4"))
	require.Equal(t, "all/claude", service.compositeBillableModel(ctx, apiKey, "all/claude", ""))
}

func TestBillableModelWithFallback(t *testing.T) {
	service := &GatewayService{billingService: NewBillingService(&config.Config{}, nil)}
	apiKey := &APIKey{}
	ctx := context.Background()

	require.Equal(t, "claude-sonnet-4", service.billableModelWithFallback(ctx, apiKey, "team/best", "", "claude-sonnet-4"))
	require.Equal(t, "claude-sonnet-4", service.billableModelWithFallback(ctx, apiKey, "claude-sonnet-4", "claude-opus-4"))
	require.Equal(t, "team/best", service.billableModelWithFallback(ctx, apiKey, "team/best", "another/alias", ""))
	require.Equal(t, "claude-sonnet-4", service.billableModelWithFallback(ctx, apiKey, "", "claude-sonnet-4"))
}

func TestHasResolvableTokenPricing(t *testing.T) {
	service := &GatewayService{billingService: NewBillingService(&config.Config{}, nil)}
	apiKey := &APIKey{}
	ctx := context.Background()

	require.True(t, service.hasResolvableTokenPricing(ctx, "claude-sonnet-4", apiKey))
	require.True(t, service.hasResolvableTokenPricing(ctx, "all/claude", apiKey))
	require.False(t, service.hasResolvableTokenPricing(ctx, "team/best", apiKey))
	require.False(t, service.hasResolvableTokenPricing(ctx, "", apiKey))
	require.False(t, (&GatewayService{}).hasResolvableTokenPricing(ctx, "claude-sonnet-4", apiKey))
}
