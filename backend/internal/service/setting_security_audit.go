package service

import (
	"context"
	"strconv"
	"strings"
)

// SettingKeyStepUpEnabled controls step-up 2FA for sensitive operations.
const SettingKeyStepUpEnabled = "step_up_enabled"

// IsSessionBindingEnabled reports whether sessions are bound to login IP and User-Agent.
// It defaults to disabled because mobile and multi-egress networks can change IPs frequently.
func (s *SettingService) IsSessionBindingEnabled(ctx context.Context) bool {
	value, err := s.settingRepo.GetValue(ctx, SettingKeySessionBindingEnabled)
	if err != nil {
		return false
	}
	return value == "true"
}

// IsStepUpEnabled reports whether sensitive operations require a recent TOTP grant.
// The switch defaults to disabled to preserve the behavior that existed before step-up gating.
func (s *SettingService) IsStepUpEnabled(ctx context.Context) bool {
	value, err := s.settingRepo.GetValue(ctx, SettingKeyStepUpEnabled)
	if err != nil {
		return false
	}
	return value == "true"
}

const defaultAuditLogRetentionDays = 180

// GetAuditLogRetentionDays returns the configured retention window; zero means permanent.
func (s *SettingService) GetAuditLogRetentionDays(ctx context.Context) int {
	value, err := s.settingRepo.GetValue(ctx, SettingKeyAuditLogRetentionDays)
	if err != nil {
		return defaultAuditLogRetentionDays
	}
	return parseAuditLogRetentionDays(value)
}

func parseAuditLogRetentionDays(value string) int {
	value = strings.TrimSpace(value)
	if value == "" {
		return defaultAuditLogRetentionDays
	}
	days, err := strconv.Atoi(value)
	if err != nil {
		return defaultAuditLogRetentionDays
	}
	if days < 0 {
		return 0
	}
	return days
}
