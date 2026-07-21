package service

import (
	"testing"

	"anlapi/internal/pkg/xai"
	"github.com/stretchr/testify/require"
)

func TestGrokQuotaFetcherBuildUsageInfoIncludesFreeTokenLimit(t *testing.T) {
	t.Parallel()

	usage := NewGrokQuotaFetcher().BuildUsageInfo(&Account{Platform: PlatformGrok, Type: AccountTypeOAuth})
	require.Equal(t, xai.GrokFreeRolling24hTokenLimit, usage.GrokFreeTokenLimit)
}
