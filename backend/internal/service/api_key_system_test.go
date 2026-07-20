package service

import (
	"context"
	"testing"
	"time"

	"anlapi/internal/config"
	"anlapi/internal/pkg/pagination"
	"github.com/stretchr/testify/require"
)

type systemImageKeyRepoStub struct {
	APIKeyRepository
	nextID      int64
	keys        map[int64]*APIKey
	bindings    map[string]int64
	ensureCalls int
}

func newSystemImageKeyRepoStub() *systemImageKeyRepoStub {
	return &systemImageKeyRepoStub{
		nextID:   1,
		keys:     make(map[int64]*APIKey),
		bindings: make(map[string]int64),
	}
}

type systemImageSettingRepoStub struct {
	SettingRepository
	values map[string]string
}

func (r *systemImageSettingRepoStub) GetValue(_ context.Context, key string) (string, error) {
	value, ok := r.values[key]
	if !ok {
		return "", ErrSettingNotFound
	}
	return value, nil
}

func newSystemImageSettingService(configuredGroupID string) (*SettingService, *systemImageSettingRepoStub) {
	repo := &systemImageSettingRepoStub{values: map[string]string{}}
	if configuredGroupID != "" {
		repo.values[SettingKeySystemImageGenerationGroupID] = configuredGroupID
	}
	return NewSettingService(repo, &config.Config{}), repo
}

func (r *systemImageKeyRepoStub) GetByID(_ context.Context, id int64) (*APIKey, error) {
	key, ok := r.keys[id]
	if !ok {
		return nil, ErrAPIKeyNotFound
	}
	clone := *key
	return &clone, nil
}

func (r *systemImageKeyRepoStub) GetKeyAndOwnerID(_ context.Context, id int64) (string, int64, error) {
	key, ok := r.keys[id]
	if !ok {
		return "", 0, ErrAPIKeyNotFound
	}
	return key.Key, key.UserID, nil
}

func (r *systemImageKeyRepoStub) Delete(_ context.Context, id int64) error {
	if _, ok := r.keys[id]; !ok {
		return ErrAPIKeyNotFound
	}
	delete(r.keys, id)
	return nil
}

func (r *systemImageKeyRepoStub) Update(_ context.Context, key *APIKey) error {
	if key == nil {
		return ErrAPIKeyNotFound
	}
	if _, ok := r.keys[key.ID]; !ok {
		return ErrAPIKeyNotFound
	}
	clone := *key
	r.keys[key.ID] = &clone
	return nil
}

func (r *systemImageKeyRepoStub) ListByUserID(_ context.Context, userID int64, params pagination.PaginationParams, _ APIKeyListFilters) ([]APIKey, *pagination.PaginationResult, error) {
	keys := make([]APIKey, 0, len(r.keys))
	for _, key := range r.keys {
		if key.UserID == userID {
			keys = append(keys, *key)
		}
	}
	return keys, &pagination.PaginationResult{Total: int64(len(keys)), Page: params.Page, PageSize: params.PageSize, Pages: 1}, nil
}

func (r *systemImageKeyRepoStub) EnsureSystemAPIKey(_ context.Context, candidate *APIKey, purpose string) (*APIKey, error) {
	if id, ok := r.bindings[purpose]; ok {
		if existing, found := r.keys[id]; found {
			clone := *existing
			clone.ManagedType = purpose
			return &clone, nil
		}
	}
	r.ensureCalls++
	clone := *candidate
	clone.ID = r.nextID
	clone.ManagedType = purpose
	clone.CreatedAt = time.Now()
	clone.UpdatedAt = clone.CreatedAt
	r.nextID++
	r.keys[clone.ID] = &clone
	r.bindings[purpose] = clone.ID
	return &clone, nil
}

func (r *systemImageKeyRepoStub) ListSystemAPIKeyPurposes(_ context.Context, userID int64) (map[int64]string, error) {
	result := make(map[int64]string)
	for purpose, id := range r.bindings {
		if key, ok := r.keys[id]; ok && key.UserID == userID {
			result[id] = purpose
		}
	}
	return result, nil
}

func (r *systemImageKeyRepoStub) GetSystemAPIKeyID(_ context.Context, userID int64, purpose string) (int64, bool, error) {
	id, ok := r.bindings[purpose]
	if !ok {
		return 0, false, nil
	}
	key, exists := r.keys[id]
	return id, exists && key.UserID == userID, nil
}

func (r *systemImageKeyRepoStub) GetSystemAPIKeyPurpose(_ context.Context, userID, keyID int64) (string, bool, error) {
	for purpose, id := range r.bindings {
		key, exists := r.keys[id]
		if id == keyID && exists && key.UserID == userID {
			return purpose, true, nil
		}
	}
	return "", false, nil
}

type systemImageKeyUserRepoStub struct {
	UserRepository
	user User
}

func (r *systemImageKeyUserRepoStub) GetByID(_ context.Context, id int64) (*User, error) {
	if id != r.user.ID {
		return nil, ErrUserNotFound
	}
	clone := r.user
	return &clone, nil
}

type systemImageKeyGroupRepoStub struct {
	GroupRepository
	groups []Group
}

func (r *systemImageKeyGroupRepoStub) ListActiveVisibleToUser(_ context.Context, _ int64, _ []int64) ([]Group, error) {
	groups := make([]Group, len(r.groups))
	copy(groups, r.groups)
	return groups, nil
}

type systemImageKeySubscriptionRepoStub struct {
	UserSubscriptionRepository
}

func (r *systemImageKeySubscriptionRepoStub) ListActiveByUserID(context.Context, int64) ([]UserSubscription, error) {
	return nil, nil
}

func TestAPIKeyService_SystemImageKeyLifecycle(t *testing.T) {
	ctx := context.Background()
	userID := int64(42)
	repo := newSystemImageKeyRepoStub()
	settingService, settingRepo := newSystemImageSettingService("10")
	groupRepo := &systemImageKeyGroupRepoStub{groups: []Group{
		{ID: 20, Name: "later image group", Platform: PlatformOpenAI, Status: StatusActive, Scope: GroupScopePublic, SubscriptionType: SubscriptionTypeStandard, AllowImageGeneration: true, SortOrder: 20},
		{ID: 10, Name: "preferred image group", Platform: PlatformOpenAI, Status: StatusActive, Scope: GroupScopePublic, SubscriptionType: SubscriptionTypeStandard, AllowImageGeneration: true, SortOrder: 10},
		{ID: 5, Name: "text only", Platform: PlatformOpenAI, Status: StatusActive, Scope: GroupScopePublic, SubscriptionType: SubscriptionTypeStandard, SortOrder: 1},
		{ID: 1, Name: "other platform image", Platform: PlatformGemini, Status: StatusActive, Scope: GroupScopePublic, SubscriptionType: SubscriptionTypeStandard, AllowImageGeneration: true, SortOrder: 0},
	}}
	svc := NewAPIKeyService(
		repo,
		&systemImageKeyUserRepoStub{user: User{ID: userID, Status: StatusActive}},
		groupRepo,
		&systemImageKeySubscriptionRepoStub{},
		nil,
		nil,
		&config.Config{},
	)
	svc.SetSettingService(settingService)

	created, err := svc.EnsureSystemImageKey(ctx, userID)
	require.NoError(t, err)
	require.Equal(t, "GPT 生图专线", created.Name)
	require.Equal(t, APIKeyManagedTypeImageGeneration, created.ManagedType)
	require.NotNil(t, created.GroupID)
	require.Equal(t, int64(10), *created.GroupID)
	require.Equal(t, 1, repo.ensureCalls)

	reused, err := svc.EnsureSystemImageKey(ctx, userID)
	require.NoError(t, err)
	require.Equal(t, created.ID, reused.ID)
	require.Equal(t, 1, repo.ensureCalls)

	settingRepo.values[SettingKeySystemImageGenerationGroupID] = "20"
	rebound, err := svc.EnsureSystemImageKey(ctx, userID)
	require.NoError(t, err)
	require.Equal(t, created.ID, rebound.ID)
	require.NotNil(t, rebound.GroupID)
	require.Equal(t, int64(20), *rebound.GroupID)
	require.Len(t, rebound.GroupRoutes, 1)
	require.Equal(t, int64(20), rebound.GroupRoutes[0].GroupID)
	require.Equal(t, 1, repo.ensureCalls)

	listed, _, err := svc.List(ctx, userID, pagination.DefaultPagination(), APIKeyListFilters{})
	require.NoError(t, err)
	require.Len(t, listed, 1)
	require.True(t, listed[0].IsSystemManaged())

	name := "should not be accepted"
	_, err = svc.Update(ctx, created.ID, userID, UpdateAPIKeyRequest{Name: &name})
	require.ErrorIs(t, err, ErrSystemAPIKeyImmutable)

	require.NoError(t, svc.Delete(ctx, created.ID, userID))
	replacementID, found, err := repo.GetSystemAPIKeyID(ctx, userID, APIKeyManagedTypeImageGeneration)
	require.NoError(t, err)
	require.True(t, found)
	require.NotEqual(t, created.ID, replacementID)
	require.Equal(t, 2, repo.ensureCalls)
}

func TestAPIKeyService_EnsureSystemImageKeyRequiresImageGroup(t *testing.T) {
	userID := int64(7)
	settingService, _ := newSystemImageSettingService("")
	svc := NewAPIKeyService(
		newSystemImageKeyRepoStub(),
		&systemImageKeyUserRepoStub{user: User{ID: userID, Status: StatusActive}},
		&systemImageKeyGroupRepoStub{groups: []Group{{ID: 1, Platform: PlatformOpenAI, Status: StatusActive, Scope: GroupScopePublic, SubscriptionType: SubscriptionTypeStandard}}},
		&systemImageKeySubscriptionRepoStub{},
		nil,
		nil,
		&config.Config{},
	)
	svc.SetSettingService(settingService)

	_, err := svc.EnsureSystemImageKey(context.Background(), userID)
	require.ErrorIs(t, err, ErrImageGenerationGroupUnavailable)
}

func TestAPIKeyService_EnsureSystemImageKeyDoesNotFallbackFromConfiguredGroup(t *testing.T) {
	userID := int64(8)
	settingService, _ := newSystemImageSettingService("20")
	svc := NewAPIKeyService(
		newSystemImageKeyRepoStub(),
		&systemImageKeyUserRepoStub{user: User{ID: userID, Status: StatusActive}},
		&systemImageKeyGroupRepoStub{groups: []Group{{
			ID:                   10,
			Platform:             PlatformOpenAI,
			Status:               StatusActive,
			Scope:                GroupScopePublic,
			SubscriptionType:     SubscriptionTypeStandard,
			AllowImageGeneration: true,
		}}},
		&systemImageKeySubscriptionRepoStub{},
		nil,
		nil,
		&config.Config{},
	)
	svc.SetSettingService(settingService)

	_, err := svc.EnsureSystemImageKey(context.Background(), userID)
	require.ErrorIs(t, err, ErrImageGenerationGroupUnavailable)
}

var _ SystemAPIKeyRepository = (*systemImageKeyRepoStub)(nil)
