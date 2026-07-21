package admin

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSystemUpdateContextDetachesCancellationAndKeepsDeadline(t *testing.T) {
	type contextKey string
	const key contextKey = "request-id"

	parent, cancelParent := context.WithCancel(context.WithValue(context.Background(), key, "req-42"))
	cancelParent()

	updateCtx, cancelUpdate := systemUpdateContext(parent)
	defer cancelUpdate()

	require.NoError(t, updateCtx.Err())
	require.Equal(t, "req-42", updateCtx.Value(key))
	deadline, ok := updateCtx.Deadline()
	require.True(t, ok)
	require.WithinDuration(t, time.Now().Add(systemUpdateTimeout), deadline, time.Second)
}
