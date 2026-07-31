//go:build unit

package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

type invalidAuthAbuseHealthStub struct {
	health InvalidAuthAbuseHealth
}

func (stub invalidAuthAbuseHealthStub) InvalidAuthAbuseHealth() InvalidAuthAbuseHealth {
	return stub.health
}

func TestOpsIngressRejectHealthIncludesInvalidAuthAbuse(t *testing.T) {
	want := InvalidAuthAbuseHealth{Enabled: true, Tracked: 3, Capacity: 16, Evicted: 2, Dropped: 1}
	aggregator := NewOpsIngressRejectAggregator(nil, invalidAuthAbuseHealthStub{health: want})
	require.Equal(t, want, aggregator.Health().InvalidAuthAbuse)
}
