//go:build unit

package service

import (
	"context"
	"testing"

	"anl-api/internal/config"

	"github.com/stretchr/testify/require"
)

func TestSecuritySwitchesDefaultDisabled(t *testing.T) {
	svc := NewSettingService(&settingValueRepoStub{values: map[string]string{}}, &config.Config{})

	require.False(t, svc.IsSessionBindingEnabled(context.Background()))
	require.False(t, svc.IsStepUpEnabled(context.Background()))
}

func TestSecuritySwitchesRequireExplicitTrue(t *testing.T) {
	repo := &settingValueRepoStub{values: map[string]string{
		SettingKeySessionBindingEnabled: "true",
		SettingKeyStepUpEnabled:         "true",
	}}
	svc := NewSettingService(repo, &config.Config{})

	require.True(t, svc.IsSessionBindingEnabled(context.Background()))
	require.True(t, svc.IsStepUpEnabled(context.Background()))
}
