package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
)

const SettingKeyRateLimit429CooldownSettings = "rate_limit_429_cooldown_settings"

type RateLimit429CooldownSettings struct {
	Enabled         bool `json:"enabled"`
	CooldownSeconds int  `json:"cooldown_seconds"`
}

func DefaultRateLimit429CooldownSettings() *RateLimit429CooldownSettings {
	return &RateLimit429CooldownSettings{Enabled: true, CooldownSeconds: 5}
}

func (s *SettingService) GetRateLimit429CooldownSettings(ctx context.Context) (*RateLimit429CooldownSettings, error) {
	value, err := s.settingRepo.GetValue(ctx, SettingKeyRateLimit429CooldownSettings)
	if err != nil {
		if errors.Is(err, ErrSettingNotFound) {
			return DefaultRateLimit429CooldownSettings(), nil
		}
		return nil, fmt.Errorf("get 429 cooldown settings: %w", err)
	}
	if value == "" {
		return DefaultRateLimit429CooldownSettings(), nil
	}

	var settings RateLimit429CooldownSettings
	if err := json.Unmarshal([]byte(value), &settings); err != nil {
		return DefaultRateLimit429CooldownSettings(), nil
	}
	if settings.CooldownSeconds < 1 {
		settings.CooldownSeconds = 1
	}
	if settings.CooldownSeconds > 7200 {
		settings.CooldownSeconds = 7200
	}
	return &settings, nil
}

func (s *SettingService) SetRateLimit429CooldownSettings(ctx context.Context, settings *RateLimit429CooldownSettings) error {
	if settings == nil {
		return fmt.Errorf("settings cannot be nil")
	}
	if settings.CooldownSeconds < 1 || settings.CooldownSeconds > 7200 {
		if settings.Enabled {
			return fmt.Errorf("cooldown_seconds must be between 1-7200")
		}
		settings.CooldownSeconds = 5
	}
	data, err := json.Marshal(settings)
	if err != nil {
		return fmt.Errorf("marshal 429 cooldown settings: %w", err)
	}
	return s.settingRepo.Set(ctx, SettingKeyRateLimit429CooldownSettings, string(data))
}
