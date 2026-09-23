package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCalculateSearchCost(t *testing.T) {
	t.Parallel()
	s := &BillingService{}
	require.Equal(t, 0.0, s.CalculateSearchCost(0, floatPtr(10), 1).ActualCost)
	// nil price -> official xAI default $5/1k: 5 calls = 0.025
	defaultCost := s.CalculateSearchCost(5, nil, 1)
	require.InDelta(t, 0.025, defaultCost.ActualCost, 1e-9)
	require.NotEmpty(t, defaultCost.PricingVersion)
	// explicit 0 → free
	freeCost := s.CalculateSearchCost(5, floatPtr(0), 1)
	require.Equal(t, 0.0, freeCost.ActualCost)
	require.NotEmpty(t, freeCost.PricingVersion)
	require.NotEqual(t, defaultCost.PricingVersion, freeCost.PricingVersion)
	price := 10.0
	cost := s.CalculateSearchCost(100, &price, 1.5)
	// 10 / 1000 * 100 * 1.5 = 1.5
	require.InDelta(t, 1.0, cost.TotalCost, 1e-9)
	require.InDelta(t, 1.5, cost.ActualCost, 1e-9)
}

func TestCalculateAudioCost(t *testing.T) {
	t.Parallel()
	s := &BillingService{}
	rt, tts, stt := 0.10, 15.0, 0.50
	cfg := &audioPriceConfig{RealtimePerMin: &rt, TTSPerMChars: &tts, STTPerHour: &stt}
	configuredRealtime := s.CalculateAudioCost("realtime", 2, cfg, 1)
	require.InDelta(t, 0.20, configuredRealtime.ActualCost, 1e-9)
	require.NotEmpty(t, configuredRealtime.PricingVersion)
	require.InDelta(t, 1.5, s.CalculateAudioCost("tts", 0.1, cfg, 1).ActualCost, 1e-9)
	require.InDelta(t, 0.25, s.CalculateAudioCost("stt", 0.5, cfg, 1).ActualCost, 1e-9)
	require.Equal(t, 0.0, s.CalculateAudioCost("unknown", 1, cfg, 1).ActualCost)
	// nil config -> official defaults (think-fast-1 $0.05/min, TTS $15/M, REST STT $0.10/hr)
	defaultRealtime := s.CalculateAudioCost("realtime", 1, nil, 1)
	require.InDelta(t, 0.05, defaultRealtime.ActualCost, 1e-9)
	require.NotEmpty(t, defaultRealtime.PricingVersion)
	require.NotEqual(t, configuredRealtime.PricingVersion, defaultRealtime.PricingVersion)
	require.InDelta(t, 15.0, s.CalculateAudioCost("tts", 1, nil, 1).ActualCost, 1e-9)
	require.InDelta(t, 0.10, s.CalculateAudioCost("stt", 1, nil, 1).ActualCost, 1e-9)
	// explicit 0 → free
	zero := 0.0
	freeRealtime := s.CalculateAudioCost("realtime", 1, &audioPriceConfig{RealtimePerMin: &zero}, 1)
	require.Equal(t, 0.0, freeRealtime.ActualCost)
	require.NotEmpty(t, freeRealtime.PricingVersion)
}

func floatPtr(v float64) *float64 { return &v }
