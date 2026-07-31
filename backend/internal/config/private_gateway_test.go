package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPrivateGatewayConfigValidate(t *testing.T) {
	valid := PrivateGatewayConfig{
		Enabled:    true,
		ServiceID:  "anl-hub-test",
		SigningKey: "test-signing-key-for-anlapi-service-123456",
		APIKeyID:   42,
	}
	require.NoError(t, valid.Validate())

	for _, mutate := range []func(*PrivateGatewayConfig){
		func(c *PrivateGatewayConfig) { c.ServiceID = "bad service id" },
		func(c *PrivateGatewayConfig) { c.SigningKey = "short" },
		func(c *PrivateGatewayConfig) { c.SigningKey = "signing key with spaces is invalid-123456" },
		func(c *PrivateGatewayConfig) { c.APIKeyID = 0 },
	} {
		candidate := valid
		mutate(&candidate)
		require.Error(t, candidate.Validate())
	}

	disabled := PrivateGatewayConfig{Enabled: false}
	require.NoError(t, disabled.Validate())
}

func TestLoadPrivateGatewayConfigFromEnv(t *testing.T) {
	resetViperWithJWTSecret(t)
	t.Setenv("PRIVATE_GATEWAY_ENABLED", "true")
	t.Setenv("PRIVATE_GATEWAY_SERVICE_ID", "anl-hub-test")
	t.Setenv("PRIVATE_GATEWAY_SIGNING_KEY", "test-signing-key-for-anlapi-service-123456")
	t.Setenv("PRIVATE_GATEWAY_API_KEY_ID", "42")

	cfg, err := Load()
	require.NoError(t, err)
	require.True(t, cfg.PrivateGateway.Enabled)
	require.Equal(t, "anl-hub-test", cfg.PrivateGateway.ServiceID)
	require.Equal(t, int64(42), cfg.PrivateGateway.APIKeyID)
}
