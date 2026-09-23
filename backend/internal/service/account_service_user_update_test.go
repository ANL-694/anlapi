package service

import (
	"context"
	"testing"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

func TestCreateOwnedUserUsesFixedDefaultsAndProtectsAPIKeySharing(t *testing.T) {
	ownerID := int64(11)
	repo := &ownedAccountServiceRepoStub{}
	svc := NewAccountService(repo, nil)
	shareMode := AccountShareModePublic

	created, err := svc.CreateOwnedUser(context.Background(), ownerID, UserAccountCreateRequest{
		Name:        "personal",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		ShareMode:   &shareMode,
		Credentials: map[string]any{"api_key": "test-key"},
	})
	require.NoError(t, err)
	require.Equal(t, ownerID, *created.OwnerUserID)
	require.Equal(t, 3, created.Concurrency)
	require.Equal(t, 1, created.Priority)
	require.True(t, created.AutoPauseOnExpired)
	require.True(t, created.Schedulable)
	require.Equal(t, StatusActive, created.Status)
	require.Equal(t, AccountShareModePrivate, created.ShareMode)
	require.Equal(t, AccountShareStatusApproved, created.ShareStatus)
}

func TestUpdateOwnedUserForcesExistingAPIKeyPrivate(t *testing.T) {
	ownerID := int64(12)
	repo := &ownedAccountServiceRepoStub{account: &Account{
		ID:          90,
		OwnerUserID: &ownerID,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		ShareMode:   AccountShareModePublic,
		ShareStatus: AccountShareStatusApproved,
	}}
	svc := NewAccountService(repo, nil)
	mode := AccountShareModePublic

	updated, err := svc.UpdateOwnedUser(context.Background(), ownerID, 90, UserAccountUpdateRequest{ShareMode: &mode})
	require.NoError(t, err)
	require.Equal(t, AccountShareModePrivate, updated.ShareMode)
	require.Equal(t, AccountShareStatusApproved, updated.ShareStatus)
}

func TestCreateOwnedUserRejectsAdminNestedSettings(t *testing.T) {
	ownerID := int64(13)
	svc := NewAccountService(&ownedAccountServiceRepoStub{}, nil)

	_, err := svc.CreateOwnedUser(context.Background(), ownerID, UserAccountCreateRequest{
		Name:        "personal",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Credentials: map[string]any{"access_token": "token", "pool_mode": true},
		Extra:       map[string]any{"email": "user@example.test", "openai_passthrough": true},
	})
	require.Error(t, err)
	require.Equal(t, "USER_ACCOUNT_FIELD_NOT_ALLOWED", infraerrors.Reason(err))
}

func TestUpdateOwnedUserAllowsUserFieldsAndPreservesManagedState(t *testing.T) {
	ownerID := int64(7)
	repo := &ownedAccountServiceRepoStub{
		account: &Account{
			ID:          44,
			OwnerUserID: &ownerID,
			Name:        "free-model",
			Platform:    PlatformOpenAI,
			Type:        AccountTypeAPIKey,
			Credentials: map[string]any{
				"api_key":   "secret-key",
				"base_url":  "https://old.example/v1",
				"pool_mode": true,
				"model_mapping": map[string]any{
					"old-model": "old-model",
				},
			},
			Extra: map[string]any{
				"quota_limit":                float64(5),
				"free_model_disabled_models": map[string]any{},
			},
		},
	}
	svc := NewAccountService(repo, nil)
	name := "renamed-free-model"
	credentials := map[string]any{
		"base_url": "https://new.example/v1",
		"model_mapping": map[string]any{
			"new-model": "new-model",
		},
		// A full-object client may echo an unchanged managed setting.
		"pool_mode": true,
	}
	extra := map[string]any{
		"quota_limit": float64(5),
		"free_model_disabled_models": map[string]any{
			"new-model": map[string]any{"error": "unavailable"},
		},
	}

	updated, err := svc.UpdateOwnedUser(context.Background(), ownerID, 44, UserAccountUpdateRequest{
		Name:        &name,
		Credentials: &credentials,
		Extra:       &extra,
	})
	require.NoError(t, err)
	require.Equal(t, name, updated.Name)
	require.Equal(t, "https://new.example/v1", updated.Credentials["base_url"])
	require.Equal(t, "secret-key", updated.Credentials["api_key"])
	require.Equal(t, true, updated.Credentials["pool_mode"])
	require.Equal(t, float64(5), updated.Extra["quota_limit"])
	require.Contains(t, updated.Extra["free_model_disabled_models"], "new-model")
}

func TestUpdateOwnedUserRejectsManagedNestedMutations(t *testing.T) {
	ownerID := int64(7)
	repo := &ownedAccountServiceRepoStub{
		account: &Account{
			ID:          45,
			OwnerUserID: &ownerID,
			Platform:    PlatformOpenAI,
			Type:        AccountTypeAPIKey,
			Credentials: map[string]any{"pool_mode": true},
			Extra:       map[string]any{"quota_limit": float64(5)},
		},
	}
	svc := NewAccountService(repo, nil)

	changedPoolMode := map[string]any{"pool_mode": false}
	_, err := svc.UpdateOwnedUser(context.Background(), ownerID, 45, UserAccountUpdateRequest{Credentials: &changedPoolMode})
	require.Error(t, err)
	require.Equal(t, "USER_ACCOUNT_FIELD_NOT_ALLOWED", infraerrors.Reason(err))

	changedQuota := map[string]any{"quota_limit": float64(99)}
	_, err = svc.UpdateOwnedUser(context.Background(), ownerID, 45, UserAccountUpdateRequest{Extra: &changedQuota})
	require.Error(t, err)
	require.Equal(t, "USER_ACCOUNT_FIELD_NOT_ALLOWED", infraerrors.Reason(err))
}

func TestUpdateOwnedUserAllowsDeepSeekAPIKeyRotation(t *testing.T) {
	ownerID := int64(7)
	repo := &ownedAccountServiceRepoStub{
		account: &Account{
			ID:          46,
			OwnerUserID: &ownerID,
			Platform:    PlatformOpenAI,
			Type:        AccountTypeAPIKey,
			Credentials: map[string]any{"api_key": "old-key", "base_url": "https://api.deepseek.com"},
		},
	}
	svc := NewAccountService(repo, nil)
	credentials := map[string]any{"api_key": "new-key", "base_url": "https://api.deepseek.com/v1"}
	updated, err := svc.UpdateOwnedUser(context.Background(), ownerID, 46, UserAccountUpdateRequest{Credentials: &credentials})
	require.NoError(t, err)
	require.Equal(t, "new-key", updated.Credentials["api_key"])
	require.Equal(t, "https://api.deepseek.com/v1", updated.Credentials["base_url"])
}
