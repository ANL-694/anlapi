package service

import (
	"context"
	"errors"
	"strconv"
	"sync"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

type systemImageAPIKeyRepoStub struct {
	APIKeyRepository
	SystemAPIKeyRepository

	mu          sync.Mutex
	keys        map[int64]*APIKey
	nextID      int64
	bindingID   int64
	bindingSet  bool
	purposeErr  error
	ensureCalls int
	updated     []*APIKey
}

type systemImagePlainAPIKeyRepoStub struct {
	APIKeyRepository
}

func (r *systemImageAPIKeyRepoStub) GetByID(_ context.Context, id int64) (*APIKey, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	key, ok := r.keys[id]
	if !ok {
		return nil, ErrAPIKeyNotFound
	}
	clone := *key
	return &clone, nil
}

func (r *systemImageAPIKeyRepoStub) Update(_ context.Context, key *APIKey, _ APIKeyUpdateFields) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	clone := *key
	r.keys[key.ID] = &clone
	r.updated = append(r.updated, &clone)
	return nil
}

func (r *systemImageAPIKeyRepoStub) GetSystemAPIKeyID(_ context.Context, _ int64, _ string) (int64, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.bindingID, r.bindingSet, nil
}

func (r *systemImageAPIKeyRepoStub) GetSystemAPIKeyPurpose(_ context.Context, _ int64, keyID int64) (string, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.purposeErr != nil {
		return "", false, r.purposeErr
	}
	returnValue := ""
	if r.bindingSet && keyID == r.bindingID {
		returnValue = APIKeyManagedTypeImageGeneration
	}
	return returnValue, returnValue != "", nil
}

func (r *systemImageAPIKeyRepoStub) ListSystemAPIKeyPurposes(_ context.Context, _ int64) (map[int64]string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.bindingSet {
		return map[int64]string{}, nil
	}
	return map[int64]string{r.bindingID: APIKeyManagedTypeImageGeneration}, nil
}

func (r *systemImageAPIKeyRepoStub) EnsureSystemAPIKey(_ context.Context, candidate *APIKey, purpose string) (*APIKey, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ensureCalls++
	if r.bindingSet {
		clone := *r.keys[r.bindingID]
		clone.ManagedType = purpose
		return &clone, nil
	}
	if r.nextID == 0 {
		r.nextID = 100
	}
	candidate.ID = r.nextID
	r.nextID++
	clone := *candidate
	clone.ManagedType = purpose
	r.keys[clone.ID] = &clone
	r.bindingID = clone.ID
	r.bindingSet = true
	return &clone, nil
}

type systemImageUserRepoStub struct {
	UserRepository
	user *User
}

func (r *systemImageUserRepoStub) GetByID(_ context.Context, _ int64) (*User, error) {
	clone := *r.user
	return &clone, nil
}

type systemImageGroupRepoStub struct {
	GroupRepository
	groups []Group
}

func (r *systemImageGroupRepoStub) ListActive(_ context.Context) ([]Group, error) {
	return append([]Group(nil), r.groups...), nil
}

func (r *systemImageGroupRepoStub) GetByID(_ context.Context, id int64) (*Group, error) {
	for i := range r.groups {
		if r.groups[i].ID == id {
			clone := r.groups[i]
			return &clone, nil
		}
	}
	return nil, ErrGroupNotFound
}

type systemImageSubscriptionRepoStub struct {
	UserSubscriptionRepository
}

func (systemImageSubscriptionRepoStub) ListActiveByUserID(context.Context, int64) ([]UserSubscription, error) {
	return nil, nil
}

type systemImageSettingRepoStub struct {
	SettingRepository
	value   string
	getErr  error
	setVals []string
}

func (r *systemImageSettingRepoStub) GetValue(context.Context, string) (string, error) {
	return r.value, r.getErr
}

func (r *systemImageSettingRepoStub) Set(_ context.Context, _, value string) error {
	r.setVals = append(r.setVals, value)
	r.value = value
	return nil
}

func newSystemImageService(groupID int64, groups []Group, repo *systemImageAPIKeyRepoStub) (*APIKeyService, *systemImageGroupRepoStub, *systemImageSettingRepoStub) {
	groupRepo := &systemImageGroupRepoStub{groups: groups}
	settingRepo := &systemImageSettingRepoStub{value: strconv.FormatInt(groupID, 10)}
	settingService := &SettingService{
		settingRepo:           settingRepo,
		defaultSubGroupReader: groupRepo,
	}
	svc := NewAPIKeyService(
		repo,
		&systemImageUserRepoStub{user: &User{ID: 7, Status: StatusActive}},
		groupRepo,
		&systemImageSubscriptionRepoStub{},
		nil,
		nil,
		&config.Config{Default: config.DefaultConfig{APIKeyPrefix: "sk-test-"}},
	)
	svc.SetSettingService(settingService)
	return svc, groupRepo, settingRepo
}

func eligibleSystemImageGroup(id int64) Group {
	return Group{
		ID:                   id,
		Platform:             PlatformOpenAI,
		Status:               StatusActive,
		Scope:                GroupScopePublic,
		AllowImageGeneration: true,
	}
}

func TestSettingServiceSystemImageGroupValidation(t *testing.T) {
	valid := eligibleSystemImageGroup(9)
	reader := &systemImageGroupRepoStub{groups: []Group{valid}}
	settings := &systemImageSettingRepoStub{}
	svc := &SettingService{settingRepo: settings, defaultSubGroupReader: reader}

	require.NoError(t, svc.SetSystemImageGenerationGroupID(context.Background(), valid.ID))
	require.Equal(t, "9", settings.value)

	tests := []struct {
		name  string
		group Group
	}{
		{name: "inactive", group: Group{ID: 10, Platform: PlatformOpenAI, Status: StatusDisabled, Scope: GroupScopePublic, AllowImageGeneration: true}},
		{name: "wrong platform", group: Group{ID: 11, Platform: PlatformAnthropic, Status: StatusActive, Scope: GroupScopePublic, AllowImageGeneration: true}},
		{name: "image disabled", group: Group{ID: 12, Platform: PlatformOpenAI, Status: StatusActive, Scope: GroupScopePublic}},
		{name: "private owner", group: func() Group {
			owner := int64(7)
			return Group{ID: 13, Platform: PlatformOpenAI, Status: StatusActive, Scope: GroupScopeUserPrivate, OwnerUserID: &owner, AllowImageGeneration: true}
		}()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader.groups = []Group{tt.group}
			err := svc.SetSystemImageGenerationGroupID(context.Background(), tt.group.ID)
			require.ErrorIs(t, err, ErrSystemImageGenerationGroupInvalid)
			require.Equal(t, 400, infraerrors.Code(err))
		})
	}
}

func TestEnsureSystemImageKeyRequiresDurableStoreAndEligibleGroup(t *testing.T) {
	group := eligibleSystemImageGroup(9)
	repo := &systemImageAPIKeyRepoStub{keys: map[int64]*APIKey{}}
	svc, _, settings := newSystemImageService(0, []Group{group}, repo)

	_, err := svc.EnsureSystemImageKey(context.Background(), 7)
	require.ErrorIs(t, err, ErrImageGenerationGroupUnavailable)
	require.Equal(t, 404, infraerrors.Code(err))

	settings.value = "9"
	key, err := svc.EnsureSystemImageKey(context.Background(), 7)
	require.NoError(t, err)
	require.Equal(t, int64(100), key.ID)
	require.Equal(t, int64(9), *key.GroupID)
	require.Equal(t, APIKeyManagedTypeImageGeneration, key.ManagedType)

	plainRepo := &systemImagePlainAPIKeyRepoStub{}
	plainSvc := NewAPIKeyService(
		plainRepo,
		&systemImageUserRepoStub{user: &User{ID: 7, Status: StatusActive}},
		&systemImageGroupRepoStub{groups: []Group{group}},
		&systemImageSubscriptionRepoStub{},
		nil,
		nil,
		&config.Config{Default: config.DefaultConfig{APIKeyPrefix: "sk-test-"}},
	)
	_, err = plainSvc.EnsureSystemImageKey(context.Background(), 7)
	require.ErrorIs(t, err, ErrSystemAPIKeyStoreUnavailable)
	require.Equal(t, 503, infraerrors.Code(err))
}

func TestEnsureSystemImageKeyReusesAndRebindsExistingKey(t *testing.T) {
	oldGroup := eligibleSystemImageGroup(9)
	newGroup := eligibleSystemImageGroup(10)
	repo := &systemImageAPIKeyRepoStub{keys: map[int64]*APIKey{}}
	svc, groupRepo, settings := newSystemImageService(oldGroup.ID, []Group{oldGroup}, repo)

	first, err := svc.EnsureSystemImageKey(context.Background(), 7)
	require.NoError(t, err)
	second, err := svc.EnsureSystemImageKey(context.Background(), 7)
	require.NoError(t, err)
	require.Equal(t, first.ID, second.ID)
	require.Equal(t, 1, repo.ensureCalls)

	groupRepo.groups = []Group{newGroup}
	settings.value = strconv.FormatInt(newGroup.ID, 10)
	rebound, err := svc.EnsureSystemImageKey(context.Background(), 7)
	require.NoError(t, err)
	require.Equal(t, first.ID, rebound.ID)
	require.Equal(t, newGroup.ID, *rebound.GroupID)
	require.Len(t, repo.updated, 1)
}

func TestAPIKeyServiceRejectsSystemKeyUpdate(t *testing.T) {
	groupID := int64(9)
	repo := &systemImageAPIKeyRepoStub{
		keys: map[int64]*APIKey{
			42: {ID: 42, UserID: 7, Key: "sk-system", GroupID: &groupID, ManagedType: APIKeyManagedTypeImageGeneration},
		},
		bindingID:  42,
		bindingSet: true,
	}
	svc := NewAPIKeyService(repo, nil, nil, nil, nil, nil, &config.Config{})

	_, err := svc.Update(context.Background(), 42, 7, UpdateAPIKeyRequest{})
	require.ErrorIs(t, err, ErrSystemAPIKeyImmutable)
	require.Equal(t, 403, infraerrors.Code(err))
	require.Empty(t, repo.updated)
}

func TestAPIKeyServiceFailsClosedWhenSystemBindingStoreIsUnavailable(t *testing.T) {
	repo := &systemImageAPIKeyRepoStub{
		keys: map[int64]*APIKey{
			42: {ID: 42, UserID: 7, Key: "sk-system"},
		},
		purposeErr: ErrSystemAPIKeyStoreUnavailable,
	}
	svc := NewAPIKeyService(repo, nil, nil, nil, nil, nil, &config.Config{})

	_, err := svc.Update(context.Background(), 42, 7, UpdateAPIKeyRequest{})
	require.Error(t, err)
	require.Equal(t, 503, infraerrors.Code(err))
	require.ErrorIs(t, err, ErrSystemAPIKeyStoreUnavailable)
	require.Empty(t, repo.updated)
}

func TestSystemAPIKeyErrorsPreserveTheirHTTPContracts(t *testing.T) {
	require.Equal(t, 403, infraerrors.Code(ErrSystemAPIKeyImmutable))
	require.Equal(t, 404, infraerrors.Code(ErrImageGenerationGroupUnavailable))
	require.Equal(t, 503, infraerrors.Code(ErrSystemAPIKeyStoreUnavailable))
	require.True(t, errors.Is(ErrSystemAPIKeyImmutable, ErrSystemAPIKeyImmutable))
}
