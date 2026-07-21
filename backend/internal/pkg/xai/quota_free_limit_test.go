package xai

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIsGrokFreeRolling24hTokenLimit(t *testing.T) {
	t.Parallel()

	require.True(t, IsGrokFreeRolling24hTokenLimit(GrokFreeRolling24hTokenLimit))
	require.True(t, IsGrokFreeRolling24hTokenLimit(2_000_000), "legacy snapshots remain classifiable")
	require.False(t, IsGrokFreeRolling24hTokenLimit(3_000_000))
}
