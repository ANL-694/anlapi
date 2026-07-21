//go:build unit

package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"anlapi/internal/config"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestConfigureTrustedProxiesFailsClosedUnlessExplicit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name string
		cfg  config.ServerConfig
		want string
	}{
		{
			name: "absent configuration ignores spoofed forwarded header",
			cfg:  config.ServerConfig{},
			want: "9.9.9.9",
		},
		{
			name: "explicit proxy CIDR resolves client address",
			cfg: config.ServerConfig{
				TrustedProxies:           []string{"9.9.9.9/32"},
				TrustedProxiesConfigured: true,
			},
			want: "1.2.3.4",
		},
		{
			name: "explicit empty configuration remains fail closed",
			cfg: config.ServerConfig{
				TrustedProxiesConfigured: true,
			},
			want: "9.9.9.9",
		},
		{
			name: "invalid explicit CIDR falls back to direct peer",
			cfg: config.ServerConfig{
				TrustedProxies:           []string{"not-a-cidr"},
				TrustedProxiesConfigured: true,
			},
			want: "9.9.9.9",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := gin.New()
			configureTrustedProxies(r, tc.cfg)
			r.GET("/t", func(c *gin.Context) {
				c.String(http.StatusOK, c.ClientIP())
			})

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/t", nil)
			req.RemoteAddr = "9.9.9.9:12345"
			req.Header.Set("X-Forwarded-For", "1.2.3.4")
			r.ServeHTTP(w, req)

			require.Equal(t, http.StatusOK, w.Code)
			require.Equal(t, tc.want, w.Body.String())
		})
	}
}
