package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

type PanelRateLimitSettings struct {
	Enabled     bool `json:"enabled"`
	UserRPM     int  `json:"user_rpm"`
	HeavyRPM    int  `json:"heavy_rpm"`
	ExemptAdmin bool `json:"exempt_admin"`
	PublicIPRPM int  `json:"public_ip_rpm"`
}

const panelRateLimitRPMMax = 100000

const (
	panelRateLimitCacheTTL  = 60 * time.Second
	panelRateLimitErrorTTL  = 5 * time.Second
	panelRateLimitDBTimeout = 5 * time.Second
)

type cachedPanelRateLimitSettings struct {
	settings  PanelRateLimitSettings
	expiresAt int64 // unix nano
}

func DefaultPanelRateLimitSettings() *PanelRateLimitSettings {
	return &PanelRateLimitSettings{
		Enabled:     true,
		UserRPM:     240,
		HeavyRPM:    60,
		ExemptAdmin: true,
		PublicIPRPM: 300,
	}
}

func normalizePanelRateLimitSettings(s *PanelRateLimitSettings) {
	if s == nil {
		return
	}
	if s.UserRPM < 0 {
		s.UserRPM = 0
	}
	if s.HeavyRPM < 0 {
		s.HeavyRPM = 0
	}
	if s.PublicIPRPM < 0 {
		s.PublicIPRPM = 0
	}
	if s.UserRPM > panelRateLimitRPMMax {
		s.UserRPM = panelRateLimitRPMMax
	}
	if s.HeavyRPM > panelRateLimitRPMMax {
		s.HeavyRPM = panelRateLimitRPMMax
	}
	if s.PublicIPRPM > panelRateLimitRPMMax {
		s.PublicIPRPM = panelRateLimitRPMMax
	}
}

func (s *SettingService) GetPanelRateLimitSettings(ctx context.Context) (*PanelRateLimitSettings, error) {
	value, err := s.settingRepo.GetValue(ctx, SettingKeyPanelRateLimitSettings)
	if err != nil {
		if errors.Is(err, ErrSettingNotFound) {
			return DefaultPanelRateLimitSettings(), nil
		}
		return nil, fmt.Errorf("get panel rate limit settings: %w", err)
	}
	if strings.TrimSpace(value) == "" {
		return DefaultPanelRateLimitSettings(), nil
	}

	settings := &PanelRateLimitSettings{}
	if err := json.Unmarshal([]byte(value), settings); err != nil {
		slog.Warn("failed to unmarshal panel rate limit settings, falling back to defaults",
			"error", err, "key", SettingKeyPanelRateLimitSettings)
		return DefaultPanelRateLimitSettings(), nil
	}
	normalizePanelRateLimitSettings(settings)
	return settings, nil
}

func (s *SettingService) SetPanelRateLimitSettings(ctx context.Context, settings *PanelRateLimitSettings) error {
	if settings == nil {
		return fmt.Errorf("settings cannot be nil")
	}
	if settings.UserRPM < 0 || settings.HeavyRPM < 0 || settings.PublicIPRPM < 0 {
		return fmt.Errorf("rate limit values cannot be negative")
	}
	if settings.UserRPM > panelRateLimitRPMMax || settings.HeavyRPM > panelRateLimitRPMMax || settings.PublicIPRPM > panelRateLimitRPMMax {
		return fmt.Errorf("rate limit values must be at most %d", panelRateLimitRPMMax)
	}

	data, err := json.Marshal(settings)
	if err != nil {
		return fmt.Errorf("marshal panel rate limit settings: %w", err)
	}
	if err := s.settingRepo.Set(ctx, SettingKeyPanelRateLimitSettings, string(data)); err != nil {
		return err
	}

	s.storePanelRateLimitCache(*settings, panelRateLimitCacheTTL)
	return nil
}

func (s *SettingService) GetPanelRateLimitSettingsCached(ctx context.Context) PanelRateLimitSettings {
	if s == nil || s.settingRepo == nil {
		return *DefaultPanelRateLimitSettings()
	}
	if cached, ok := s.panelRateLimitCache.Load().(*cachedPanelRateLimitSettings); ok && cached != nil {
		if time.Now().UnixNano() < cached.expiresAt {
			return cached.settings
		}
	}

	result, _, _ := s.panelRateLimitSF.Do("panel_rate_limit_settings", func() (any, error) {
		if cached, ok := s.panelRateLimitCache.Load().(*cachedPanelRateLimitSettings); ok && cached != nil {
			if time.Now().UnixNano() < cached.expiresAt {
				return cached.settings, nil
			}
		}
		if ctx == nil {
			ctx = context.Background()
		}
		dbCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), panelRateLimitDBTimeout)
		defer cancel()

		settings, err := s.GetPanelRateLimitSettings(dbCtx)
		if err != nil {
			slog.Warn("failed to get panel rate limit settings", "error", err)
			fallback := *DefaultPanelRateLimitSettings()
			if prior, ok := s.panelRateLimitCache.Load().(*cachedPanelRateLimitSettings); ok && prior != nil {
				fallback = prior.settings
			}
			s.storePanelRateLimitCache(fallback, panelRateLimitErrorTTL)
			return fallback, nil
		}

		s.storePanelRateLimitCache(*settings, panelRateLimitCacheTTL)
		return *settings, nil
	})
	if settings, ok := result.(PanelRateLimitSettings); ok {
		return settings
	}
	return *DefaultPanelRateLimitSettings()
}

func (s *SettingService) storePanelRateLimitCache(settings PanelRateLimitSettings, ttl time.Duration) {
	s.panelRateLimitCache.Store(&cachedPanelRateLimitSettings{
		settings:  settings,
		expiresAt: time.Now().Add(ttl).UnixNano(),
	})
}
