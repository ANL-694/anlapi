package service

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOpsSystemMetricsSnapshotDoesNotExposeConcurrencyQueueDepth(t *testing.T) {
	depth := 7
	payload, err := json.Marshal(OpsSystemMetricsSnapshot{ConcurrencyQueueDepth: &depth})
	require.NoError(t, err)
	require.NotContains(t, string(payload), "concurrency_queue_depth")
}
