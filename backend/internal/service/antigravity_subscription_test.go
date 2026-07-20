package service

import (
	"testing"

	"anl-api/internal/pkg/antigravity"
	"github.com/stretchr/testify/assert"
)

func TestNormalizeAntigravitySubscription_PaidTierWithIneligible(t *testing.T) {
	resp := &antigravity.LoadCodeAssistResponse{
		PaidTier:        &antigravity.PaidTierInfo{ID: "g1-pro-tier"},
		IneligibleTiers: []*antigravity.IneligibleTier{{ReasonMessage: "location validation required"}},
	}
	result := NormalizeAntigravitySubscription(resp)
	assert.Equal(t, "Pro", result.PlanType)
	assert.Equal(t, "abnormal", result.SubscriptionStatus)
	assert.Equal(t, "location validation required", result.SubscriptionError)
}

func TestNormalizeAntigravitySubscription_FreeTierWithIneligible(t *testing.T) {
	resp := &antigravity.LoadCodeAssistResponse{
		PaidTier:        &antigravity.PaidTierInfo{ID: "free-tier"},
		IneligibleTiers: []*antigravity.IneligibleTier{{ReasonMessage: "some warning"}},
	}
	result := NormalizeAntigravitySubscription(resp)
	assert.Equal(t, "Abnormal", result.PlanType)
	assert.Equal(t, "abnormal", result.SubscriptionStatus)
}

func TestNormalizeAntigravitySubscription_NoIneligible(t *testing.T) {
	result := NormalizeAntigravitySubscription(&antigravity.LoadCodeAssistResponse{
		PaidTier: &antigravity.PaidTierInfo{ID: "g1-ultra-tier"},
	})
	assert.Equal(t, "Ultra", result.PlanType)
	assert.Empty(t, result.SubscriptionStatus)
}

func TestNormalizeAntigravitySubscription_NilResponse(t *testing.T) {
	assert.Equal(t, "Free", NormalizeAntigravitySubscription(nil).PlanType)
}
