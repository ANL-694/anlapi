package service

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"
)

// PricingVersionPrefix identifies the versioned pricing snapshot format.
// The hash is intentionally derived from the effective prices and their source,
// not from a wall-clock timestamp, so retries made under the same price card
// produce the same audit value.
const PricingVersionPrefix = "pv1-"

type pricingSnapshot struct {
	Schema                 string
	Model                  string
	Source                 string
	Mode                   BillingMode
	EffectivePricing       *ModelPricing
	BasePricing            *ModelPricing
	Intervals              []PricingInterval
	RequestTiers           []PricingInterval
	DefaultPerRequestPrice float64
	SupportsCacheBreakdown bool
	LongContextEnabled     bool
	ChannelPricing         *ChannelModelPricing
	TimeMultiplier         float64
}

// BuildAuxiliaryPricingSnapshotVersion records a stable price card for
// non-token charges (image, video, audio, and search). These paths do not
// always have a ModelPricing object, but they still need an auditable version
// when their configured unit price changes.
func BuildAuxiliaryPricingSnapshotVersion(model string, mode BillingMode, source, tier string, unitPrice float64) string {
	price := unitPrice
	resolved := &ResolvedPricing{
		Mode:   mode,
		Source: strings.ToLower(strings.TrimSpace(source)),
		RequestTiers: []PricingInterval{{
			TierLabel:       strings.TrimSpace(tier),
			PerRequestPrice: &price,
		}},
	}
	return BuildPricingSnapshotVersion(model, source, mode, nil, resolved, 1)
}

// CombinePricingSnapshotVersions creates a stable composite version for one
// request that uses multiple price cards, such as token usage plus a search
// surcharge. Sorting makes the result independent of merge order while
// retaining every non-empty component version.
func CombinePricingSnapshotVersions(versions ...string) string {
	items := make([]string, 0, len(versions))
	for _, version := range versions {
		if version = strings.TrimSpace(version); version != "" {
			items = append(items, version)
		}
	}
	if len(items) == 0 {
		return ""
	}
	sort.Strings(items)
	payload := struct {
		Schema   string   `json:"schema"`
		Versions []string `json:"versions"`
	}{Schema: PricingVersionPrefix + "composite", Versions: items}
	data, err := json.Marshal(payload)
	if err != nil {
		return ""
	}
	digest := sha256.Sum256(data)
	return PricingVersionPrefix + hex.EncodeToString(digest[:])
}

// BuildPricingSnapshotVersion returns a stable identifier for one effective
// pricing card. It is safe to persist as an audit field and deliberately avoids
// exposing raw provider configuration or payment information.
func BuildPricingSnapshotVersion(model, source string, mode BillingMode, effective *ModelPricing, resolved *ResolvedPricing, timeMultiplier float64) string {
	if effective == nil && resolved == nil {
		return ""
	}

	snapshot := pricingSnapshot{
		Schema:           PricingVersionPrefix,
		Model:            strings.ToLower(strings.TrimSpace(model)),
		Source:           strings.ToLower(strings.TrimSpace(source)),
		Mode:             mode,
		EffectivePricing: effective,
		TimeMultiplier:   timeMultiplier,
	}
	if resolved != nil {
		snapshot.BasePricing = resolved.BasePricing
		snapshot.Intervals = resolved.Intervals
		snapshot.RequestTiers = resolved.RequestTiers
		snapshot.DefaultPerRequestPrice = resolved.DefaultPerRequestPrice
		snapshot.SupportsCacheBreakdown = resolved.SupportsCacheBreakdown
		snapshot.LongContextEnabled = resolved.longContextPricingEnabled
		snapshot.ChannelPricing = resolved.channelPricing
		if snapshot.Source == "" {
			snapshot.Source = strings.ToLower(strings.TrimSpace(resolved.Source))
		}
		if snapshot.Mode == "" {
			snapshot.Mode = resolved.Mode
		}
	}

	data, err := json.Marshal(snapshot)
	if err != nil {
		// All snapshot fields are JSON-compatible value types. Keep the failure
		// closed if a future field violates that assumption.
		return ""
	}
	digest := sha256.Sum256(data)
	return PricingVersionPrefix + hex.EncodeToString(digest[:])
}

// BuildLegacyPricingSnapshotVersion records the effective card for callers
// that still use the compatibility billing path without a ResolvedPricing.
func BuildLegacyPricingSnapshotVersion(model string, pricing *ModelPricing) string {
	return BuildPricingSnapshotVersion(model, "legacy", BillingModeToken, pricing, nil, 1)
}
