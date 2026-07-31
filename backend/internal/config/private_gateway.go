package config

import (
	"fmt"
	"regexp"
	"strings"
)

var privateGatewayServiceIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{2,127}$`)

// PrivateGatewayConfig configures Hub-to-anlapi service authentication.
// APIKeyID is a server-side billing and routing subject; it is never sent by Hub.
type PrivateGatewayConfig struct {
	Enabled    bool   `mapstructure:"enabled"`
	ServiceID  string `mapstructure:"service_id"`
	SigningKey string `mapstructure:"signing_key"`
	APIKeyID   int64  `mapstructure:"api_key_id"`
}

func (c PrivateGatewayConfig) Validate() error {
	if !c.Enabled {
		return nil
	}
	if c.ServiceID != strings.TrimSpace(c.ServiceID) || !privateGatewayServiceIDPattern.MatchString(c.ServiceID) {
		return fmt.Errorf("private_gateway.service_id is invalid")
	}
	if len(c.SigningKey) < 32 || len(c.SigningKey) > 8192 || strings.ContainsAny(c.SigningKey, " \t\r\n") {
		return fmt.Errorf("private_gateway.signing_key must be 32-8192 non-whitespace bytes")
	}
	if c.APIKeyID <= 0 {
		return fmt.Errorf("private_gateway.api_key_id must be positive")
	}
	return nil
}
