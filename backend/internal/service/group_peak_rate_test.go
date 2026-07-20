package service

import (
	"testing"
	"time"

	"anlapi/internal/pkg/timezone"
)

func TestPeakMultiplierAtBoundaries(t *testing.T) {
	if err := timezone.Init("UTC"); err != nil {
		t.Fatalf("init timezone: %v", err)
	}
	group := &Group{
		SubscriptionType:   SubscriptionTypeSubscription,
		PeakRateEnabled:    true,
		PeakStart:          "14:00",
		PeakEnd:            "18:00",
		PeakRateMultiplier: 3,
	}
	at := func(hour, minute int) time.Time {
		return time.Date(2026, 6, 29, hour, minute, 0, 0, time.UTC)
	}

	cases := []struct {
		name string
		now  time.Time
		want float64
	}{
		{name: "before", now: at(13, 59), want: 1},
		{name: "start inclusive", now: at(14, 0), want: 3},
		{name: "inside", now: at(17, 59), want: 3},
		{name: "end exclusive", now: at(18, 0), want: 1},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if got := group.PeakMultiplierAt(test.now); got != test.want {
				t.Fatalf("PeakMultiplierAt() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestValidatePeakRateConfig(t *testing.T) {
	cases := []struct {
		name     string
		typeName string
		enabled  bool
		start    string
		end      string
		value    float64
		wantErr  bool
	}{
		{name: "disabled", typeName: SubscriptionTypeStandard},
		{name: "valid", typeName: SubscriptionTypeSubscription, enabled: true, start: "14:00", end: "18:00", value: 2},
		{name: "standard rejected", typeName: SubscriptionTypeStandard, enabled: true, start: "14:00", end: "18:00", value: 2, wantErr: true},
		{name: "missing start", typeName: SubscriptionTypeSubscription, enabled: true, end: "18:00", value: 2, wantErr: true},
		{name: "cross day rejected", typeName: SubscriptionTypeSubscription, enabled: true, start: "22:00", end: "02:00", value: 2, wantErr: true},
		{name: "negative rejected", typeName: SubscriptionTypeSubscription, enabled: true, start: "14:00", end: "18:00", value: -1, wantErr: true},
		{name: "zero allowed", typeName: SubscriptionTypeSubscription, enabled: true, start: "14:00", end: "18:00", value: 0},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			err := ValidatePeakRateConfig(test.typeName, test.enabled, test.start, test.end, test.value)
			if (err != nil) != test.wantErr {
				t.Fatalf("ValidatePeakRateConfig() error = %v, wantErr %v", err, test.wantErr)
			}
		})
	}
}

func TestNormalizePeakRateConfig(t *testing.T) {
	enabled, start, end, multiplier := NormalizePeakRateConfig(
		SubscriptionTypeStandard,
		true,
		"14:00",
		"18:00",
		3,
	)
	if enabled || start != "" || end != "" || multiplier != 1 {
		t.Fatalf("standard group peak config not cleared: %v %q %q %v", enabled, start, end, multiplier)
	}
}
