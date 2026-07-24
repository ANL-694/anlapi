package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoadOpenAIProxyStreamCircuitDefaults(t *testing.T) {
	resetViperWithJWTSecret(t)

	config, err := Load()
	require.NoError(t, err)
	require.Equal(t, 2, config.Gateway.OpenAIProxyStreamCircuit.FailureThreshold)
	require.Equal(t, 60, config.Gateway.OpenAIProxyStreamCircuit.WindowSeconds)
	require.Equal(t, 600, config.Gateway.OpenAIProxyStreamCircuit.TTLSeconds)
}

func TestLoadOpenAIProxyStreamCircuitFromEnvironment(t *testing.T) {
	resetViperWithJWTSecret(t)
	t.Setenv("GATEWAY_OPENAI_PROXY_STREAM_CIRCUIT_FAILURE_THRESHOLD", "3")
	t.Setenv("GATEWAY_OPENAI_PROXY_STREAM_CIRCUIT_WINDOW_SECONDS", "90")
	t.Setenv("GATEWAY_OPENAI_PROXY_STREAM_CIRCUIT_TTL_SECONDS", "420")

	config, err := Load()
	require.NoError(t, err)
	require.Equal(t, 3, config.Gateway.OpenAIProxyStreamCircuit.FailureThreshold)
	require.Equal(t, 90, config.Gateway.OpenAIProxyStreamCircuit.WindowSeconds)
	require.Equal(t, 420, config.Gateway.OpenAIProxyStreamCircuit.TTLSeconds)
}

func TestValidateOpenAIProxyStreamCircuitRejectsNegativeValues(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*Config)
		wantErr string
	}{
		{
			name: "failure threshold",
			mutate: func(config *Config) {
				config.Gateway.OpenAIProxyStreamCircuit.FailureThreshold = -1
			},
			wantErr: "gateway.openai_proxy_stream_circuit.failure_threshold",
		},
		{
			name: "window seconds",
			mutate: func(config *Config) {
				config.Gateway.OpenAIProxyStreamCircuit.WindowSeconds = -1
			},
			wantErr: "gateway.openai_proxy_stream_circuit.window_seconds",
		},
		{
			name: "ttl seconds",
			mutate: func(config *Config) {
				config.Gateway.OpenAIProxyStreamCircuit.TTLSeconds = -1
			},
			wantErr: "gateway.openai_proxy_stream_circuit.ttl_seconds",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			resetViperWithJWTSecret(t)
			config, err := Load()
			require.NoError(t, err)
			test.mutate(config)
			require.ErrorContains(t, config.Validate(), test.wantErr)
		})
	}
}
