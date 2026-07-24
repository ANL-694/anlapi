package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAlipayMobilePrecreateDeepLinkMigrationIsOptInAndIdempotent(t *testing.T) {
	content, err := FS.ReadFile("203_alipay_mobile_precreate_deep_link.sql")
	require.NoError(t, err)

	sql := strings.Join(strings.Fields(string(content)), " ")
	require.Contains(t, sql, "'ALIPAY_MOBILE_PRECREATE_DEEP_LINK', 'false'")
	require.Contains(t, sql, "ON CONFLICT (key) DO NOTHING")
}
