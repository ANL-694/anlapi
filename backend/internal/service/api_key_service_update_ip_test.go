package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func newAPIKeyUpdateIPTestService() (*APIKeyService, *systemImageKeyRepoStub, int64) {
	const userID int64 = 42
	repo := newSystemImageKeyRepoStub()
	repo.keys[1] = &APIKey{
		ID:          1,
		UserID:      userID,
		Key:         "sk-update-ip-test",
		Name:        "before",
		Status:      StatusActive,
		IPWhitelist: []string{"192.0.2.10"},
		IPBlacklist: []string{"198.51.100.0/24"},
	}
	return &APIKeyService{apiKeyRepo: repo}, repo, userID
}

func TestAPIKeyServiceUpdateKeepsIPRulesWhenFieldsAreOmitted(t *testing.T) {
	svc, repo, userID := newAPIKeyUpdateIPTestService()
	name := "after"

	updated, err := svc.Update(context.Background(), 1, userID, UpdateAPIKeyRequest{Name: &name})

	require.NoError(t, err)
	require.Equal(t, []string{"192.0.2.10"}, updated.IPWhitelist)
	require.Equal(t, []string{"198.51.100.0/24"}, updated.IPBlacklist)
	require.Equal(t, updated.IPWhitelist, repo.keys[1].IPWhitelist)
	require.Equal(t, updated.IPBlacklist, repo.keys[1].IPBlacklist)
}

func TestAPIKeyServiceUpdateClearsExplicitEmptyIPRules(t *testing.T) {
	svc, _, userID := newAPIKeyUpdateIPTestService()
	empty := []string{}

	updated, err := svc.Update(context.Background(), 1, userID, UpdateAPIKeyRequest{
		IPWhitelist: &empty,
		IPBlacklist: &empty,
	})

	require.NoError(t, err)
	require.Empty(t, updated.IPWhitelist)
	require.Empty(t, updated.IPBlacklist)
}

func TestAPIKeyServiceUpdateReplacesAndValidatesIPRules(t *testing.T) {
	svc, _, userID := newAPIKeyUpdateIPTestService()
	whitelist := []string{"203.0.113.7", "2001:db8::/32"}
	blacklist := []string{"10.0.0.0/8"}

	updated, err := svc.Update(context.Background(), 1, userID, UpdateAPIKeyRequest{
		IPWhitelist: &whitelist,
		IPBlacklist: &blacklist,
	})

	require.NoError(t, err)
	require.Equal(t, whitelist, updated.IPWhitelist)
	require.Equal(t, blacklist, updated.IPBlacklist)

	invalid := []string{"not-an-ip"}
	_, err = svc.Update(context.Background(), 1, userID, UpdateAPIKeyRequest{IPWhitelist: &invalid})
	require.ErrorIs(t, err, ErrInvalidIPPattern)
}
