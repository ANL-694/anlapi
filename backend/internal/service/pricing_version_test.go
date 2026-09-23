package service

import "testing"

func TestBuildPricingSnapshotVersionIsStableAndChangesWithEffectivePrice(t *testing.T) {
	pricing := &ModelPricing{InputPricePerToken: 1e-6, OutputPricePerToken: 2e-6}
	first := BuildPricingSnapshotVersion("gpt-5.6-sol", PricingSourceLiteLLM, BillingModeToken, pricing, nil, 1)
	second := BuildPricingSnapshotVersion("gpt-5.6-sol", PricingSourceLiteLLM, BillingModeToken, pricing, nil, 1)
	if first == "" || first != second {
		t.Fatalf("expected stable non-empty pricing version, first=%q second=%q", first, second)
	}

	changed := *pricing
	changed.OutputPricePerToken = 3e-6
	third := BuildPricingSnapshotVersion("gpt-5.6-sol", PricingSourceLiteLLM, BillingModeToken, &changed, nil, 1)
	if third == first {
		t.Fatalf("expected effective price change to change version: %q", first)
	}
}

func TestBuildPricingSnapshotVersionIncludesSourceAndTimeMultiplier(t *testing.T) {
	pricing := &ModelPricing{InputPricePerToken: 1e-6}
	base := BuildPricingSnapshotVersion("model", PricingSourceLiteLLM, BillingModeToken, pricing, nil, 1)
	channel := BuildPricingSnapshotVersion("model", PricingSourceChannel, BillingModeToken, pricing, nil, 1)
	peak := BuildPricingSnapshotVersion("model", PricingSourceLiteLLM, BillingModeToken, pricing, nil, 2)
	if base == channel || base == peak || channel == peak {
		t.Fatalf("expected source and time multiplier to participate in snapshot version")
	}
}

func TestBuildAuxiliaryPricingSnapshotVersionIsStableAndTracksUnitPrice(t *testing.T) {
	first := BuildAuxiliaryPricingSnapshotVersion("grok-imagine-video", BillingModeVideo, PricingSourceFallback, "720p", 0.05)
	second := BuildAuxiliaryPricingSnapshotVersion("grok-imagine-video", BillingModeVideo, PricingSourceFallback, "720p", 0.05)
	if first == "" || first != second {
		t.Fatalf("expected stable auxiliary pricing version, first=%q second=%q", first, second)
	}
	changed := BuildAuxiliaryPricingSnapshotVersion("grok-imagine-video", BillingModeVideo, PricingSourceFallback, "720p", 0.06)
	if changed == first {
		t.Fatalf("expected unit price change to change version: %q", first)
	}
}

func TestCombinePricingSnapshotVersionsIsOrderIndependent(t *testing.T) {
	a := BuildAuxiliaryPricingSnapshotVersion("", BillingModePerRequest, PricingSourceFallback, "search_per_1k", 5)
	b := BuildAuxiliaryPricingSnapshotVersion("gpt-5.6-sol", BillingModeToken, PricingSourceLiteLLM, "", 0)
	first := CombinePricingSnapshotVersions(a, b)
	second := CombinePricingSnapshotVersions(b, a)
	if first == "" || first != second {
		t.Fatalf("expected order-independent composite version, first=%q second=%q", first, second)
	}
	changed := CombinePricingSnapshotVersions(a, BuildAuxiliaryPricingSnapshotVersion("gpt-5.6-sol", BillingModeToken, PricingSourceLiteLLM, "", 1))
	if changed == first {
		t.Fatalf("expected component version change to change composite version: %q", first)
	}
}

func TestUsageBillingCommandNormalizeTrimsPricingVersion(t *testing.T) {
	cmd := &UsageBillingCommand{RequestID: "req", PricingVersion: "  pv1-example  "}
	cmd.Normalize()
	if cmd.PricingVersion != "pv1-example" {
		t.Fatalf("expected normalized pricing version, got %q", cmd.PricingVersion)
	}
}
