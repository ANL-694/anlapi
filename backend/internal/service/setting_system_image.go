package service

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

var ErrSystemImageGenerationGroupInvalid = infraerrors.BadRequest(
	"SYSTEM_IMAGE_GENERATION_GROUP_INVALID",
	"system image generation group must be an active public OpenAI image group",
)

// GetSystemImageGenerationGroupID returns the administrator-selected group.
// Missing configuration is represented by zero; malformed persisted values
// fail closed instead of silently selecting a fallback group.
func (s *SettingService) GetSystemImageGenerationGroupID(ctx context.Context) (int64, error) {
	if s == nil || s.settingRepo == nil {
		return 0, nil
	}
	raw, err := s.settingRepo.GetValue(ctx, SettingKeySystemImageGenerationGroupID)
	if err != nil {
		if errors.Is(err, ErrSettingNotFound) {
			return 0, nil
		}
		return 0, fmt.Errorf("get system image generation group setting: %w", err)
	}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, nil
	}
	groupID, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || groupID < 0 {
		return 0, fmt.Errorf("%w: invalid group id", ErrSystemImageGenerationGroupInvalid)
	}
	return groupID, nil
}

// SetSystemImageGenerationGroupID validates and persists the administrator's
// selected public OpenAI image group. Zero explicitly disables the feature.
func (s *SettingService) SetSystemImageGenerationGroupID(ctx context.Context, groupID int64) error {
	if s == nil || s.settingRepo == nil {
		return fmt.Errorf("%w: setting repository unavailable", ErrSystemImageGenerationGroupInvalid)
	}
	if groupID < 0 {
		return ErrSystemImageGenerationGroupInvalid
	}
	if groupID > 0 {
		if s.defaultSubGroupReader == nil {
			return fmt.Errorf("%w: group reader unavailable", ErrSystemImageGenerationGroupInvalid)
		}
		group, err := s.defaultSubGroupReader.GetByID(ctx, groupID)
		if err != nil {
			if errors.Is(err, ErrGroupNotFound) {
				return ErrSystemImageGenerationGroupInvalid
			}
			return fmt.Errorf("validate system image generation group: %w", err)
		}
		if group == nil || !group.IsActive() ||
			!strings.EqualFold(strings.TrimSpace(group.Platform), PlatformOpenAI) ||
			!group.AllowImageGeneration || group.OwnerUserID != nil ||
			NormalizeGroupScope(group.Scope) != GroupScopePublic {
			return ErrSystemImageGenerationGroupInvalid
		}
	}
	if err := s.settingRepo.Set(ctx, SettingKeySystemImageGenerationGroupID, strconv.FormatInt(groupID, 10)); err != nil {
		return fmt.Errorf("set system image generation group: %w", err)
	}
	return nil
}
