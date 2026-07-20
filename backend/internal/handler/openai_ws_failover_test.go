package handler

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	"anlapi/internal/service"
	"github.com/stretchr/testify/require"
)

func TestOpenAIWSProxyFailoverError(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		canRetry   bool
		statusCode int
	}{
		{
			name: "first turn failover can switch account",
			err: &service.UpstreamFailoverError{
				StatusCode:        http.StatusBadGateway,
				NextAccountAction: service.NextAccountRetry,
			},
			canRetry:   true,
			statusCode: http.StatusBadGateway,
		},
		{
			name: "wrapped failover is still detected",
			err: fmt.Errorf("proxy: %w", &service.UpstreamFailoverError{
				StatusCode:        http.StatusTooManyRequests,
				NextAccountAction: service.NextAccountRetry,
			}),
			canRetry:   true,
			statusCode: http.StatusTooManyRequests,
		},
		{
			name: "partial response cannot be replayed",
			err: &service.UpstreamFailoverError{
				StatusCode:               http.StatusBadGateway,
				NextAccountAction:        service.NextAccountRetry,
				SafeToFailoverAfterWrite: true,
			},
			canRetry:   false,
			statusCode: http.StatusBadGateway,
		},
		{
			name: "stop action cannot switch account",
			err: &service.UpstreamFailoverError{
				StatusCode:        http.StatusBadGateway,
				NextAccountAction: service.NextAccountStop,
			},
			canRetry:   false,
			statusCode: http.StatusBadGateway,
		},
		{
			name:     "ordinary error is not failover",
			err:      errors.New("upstream failed"),
			canRetry: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			failoverErr, canRetry := openAIWSProxyFailoverError(tt.err)
			require.Equal(t, tt.canRetry, canRetry)
			if tt.statusCode == 0 {
				require.Nil(t, failoverErr)
				return
			}
			require.NotNil(t, failoverErr)
			require.Equal(t, tt.statusCode, failoverErr.StatusCode)
		})
	}
}
