package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

var (
	ErrSystemAPIKeyImmutable = infraerrors.Forbidden(
		"SYSTEM_API_KEY_IMMUTABLE",
		"system-managed API keys cannot be modified",
	)
	ErrImageGenerationGroupUnavailable = infraerrors.NotFound(
		"IMAGE_GENERATION_GROUP_UNAVAILABLE",
		"the configured image generation group is unavailable",
	)
	// ErrSystemAPIKeyStoreUnavailable is kept separate from ordinary API-key
	// errors so legacy schemas can continue listing user keys while the
	// explicit system-key endpoint fails closed until migration is applied.
	ErrSystemAPIKeyStoreUnavailable = infraerrors.ServiceUnavailable(
		"SYSTEM_API_KEY_STORE_UNAVAILABLE",
		"system-managed API key storage is unavailable",
	)
)

// SystemAPIKeyRepository is an optional capability implemented by the SQL
// repository. Keeping it separate avoids widening every API-key test double.
type SystemAPIKeyRepository interface {
	EnsureSystemAPIKey(ctx context.Context, candidate *APIKey, purpose string) (*APIKey, error)
	ListSystemAPIKeyPurposes(ctx context.Context, userID int64) (map[int64]string, error)
	GetSystemAPIKeyID(ctx context.Context, userID int64, purpose string) (int64, bool, error)
	GetSystemAPIKeyPurpose(ctx context.Context, userID, keyID int64) (string, bool, error)
}

// SetSettingService attaches the setting source used by system-managed key
// provisioning without changing the constructor signature used by tests.
func (s *APIKeyService) SetSettingService(settingService *SettingService) {
	if s != nil {
		s.settingService = settingService
	}
}

func (s *APIKeyService) systemAPIKeyRepository() (SystemAPIKeyRepository, bool) {
	if s == nil || s.apiKeyRepo == nil {
		return nil, false
	}
	repo, ok := s.apiKeyRepo.(SystemAPIKeyRepository)
	return repo, ok
}

func isIgnorableSystemAPIKeyError(err error) bool {
	return errors.Is(err, ErrSystemAPIKeyStoreUnavailable) ||
		errors.Is(err, ErrImageGenerationGroupUnavailable)
}

func (s *APIKeyService) annotateSystemAPIKey(ctx context.Context, key *APIKey) error {
	if key == nil {
		return nil
	}
	repo, ok := s.systemAPIKeyRepository()
	if !ok {
		return nil
	}
	purpose, found, err := repo.GetSystemAPIKeyPurpose(ctx, key.UserID, key.ID)
	if err != nil {
		return err
	}
	if found {
		key.ManagedType = purpose
	}
	return nil
}

func (s *APIKeyService) annotateSystemAPIKeys(ctx context.Context, userID int64, keys []APIKey) error {
	if len(keys) == 0 {
		return nil
	}
	repo, ok := s.systemAPIKeyRepository()
	if !ok {
		return nil
	}
	purposes, err := repo.ListSystemAPIKeyPurposes(ctx, userID)
	if err != nil {
		return err
	}
	for i := range keys {
		keys[i].ManagedType = purposes[keys[i].ID]
	}
	return nil
}

func (s *APIKeyService) systemImageGenerationGroup(ctx context.Context, userID int64) (*Group, error) {
	if s == nil || s.settingService == nil || userID <= 0 {
		return nil, ErrImageGenerationGroupUnavailable
	}
	configuredGroupID, err := s.settingService.GetSystemImageGenerationGroupID(ctx)
	if err != nil {
		return nil, fmt.Errorf("get system image generation group: %w", err)
	}
	if configuredGroupID <= 0 {
		return nil, ErrImageGenerationGroupUnavailable
	}

	groups, err := s.GetAvailableGroups(ctx, userID)
	if err != nil {
		return nil, err
	}
	for i := range groups {
		group := &groups[i]
		if group.ID != configuredGroupID || !group.IsActive() ||
			!strings.EqualFold(strings.TrimSpace(group.Platform), PlatformOpenAI) ||
			!group.AllowImageGeneration || group.OwnerUserID != nil ||
			NormalizeGroupScope(group.Scope) != GroupScopePublic {
			continue
		}
		return group, nil
	}
	return nil, ErrImageGenerationGroupUnavailable
}

func bindSystemImageKeyToGroup(key *APIKey, group *Group) {
	if key == nil || group == nil {
		return
	}
	groupID := group.ID
	key.Name = "GPT 生图专线"
	key.GroupID = &groupID
	key.Group = group
	key.Status = StatusAPIKeyActive
}

// EnsureSystemImageKey returns the user's single durable platform-managed
// image key. The repository's unique constraint provides the cross-instance
// single-winner guarantee; this method only selects and validates the group.
func (s *APIKeyService) EnsureSystemImageKey(ctx context.Context, userID int64) (*APIKey, error) {
	repo, ok := s.systemAPIKeyRepository()
	if !ok {
		return nil, ErrSystemAPIKeyStoreUnavailable
	}
	group, groupErr := s.systemImageGenerationGroup(ctx, userID)

	if keyID, found, err := repo.GetSystemAPIKeyID(ctx, userID, APIKeyManagedTypeImageGeneration); err != nil {
		if !errors.Is(err, ErrSystemAPIKeyStoreUnavailable) {
			return nil, err
		}
	} else if found {
		existing, getErr := s.apiKeyRepo.GetByID(ctx, keyID)
		if getErr == nil && existing != nil && existing.UserID == userID {
			if groupErr != nil {
				return nil, groupErr
			}
			needsUpdate := existing.Name != "GPT 生图专线" ||
				existing.GroupID == nil || *existing.GroupID != group.ID ||
				existing.Status != StatusAPIKeyActive
			if needsUpdate {
				bindSystemImageKeyToGroup(existing, group)
				if err := s.apiKeyRepo.Update(ctx, existing, APIKeyUpdateFields{Name: true, GroupID: true, Status: true}); err != nil {
					return nil, fmt.Errorf("rebind system image api key: %w", err)
				}
				s.InvalidateAuthCacheByKey(ctx, existing.Key)
			}
			existing.ManagedType = APIKeyManagedTypeImageGeneration
			s.compileAPIKeyIPRules(existing)
			return existing, nil
		}
		if getErr != nil && !errors.Is(getErr, ErrAPIKeyNotFound) {
			return nil, getErr
		}
	}

	if groupErr != nil {
		return nil, groupErr
	}
	generatedKey, err := s.GenerateKey()
	if err != nil {
		return nil, fmt.Errorf("generate system image api key: %w", err)
	}
	candidate := &APIKey{UserID: userID, Key: generatedKey}
	bindSystemImageKeyToGroup(candidate, group)
	created, err := repo.EnsureSystemAPIKey(ctx, candidate, APIKeyManagedTypeImageGeneration)
	if err != nil {
		return nil, err
	}
	created.ManagedType = APIKeyManagedTypeImageGeneration
	s.InvalidateAuthCacheByKey(ctx, created.Key)
	s.compileAPIKeyIPRules(created)
	return created, nil
}
