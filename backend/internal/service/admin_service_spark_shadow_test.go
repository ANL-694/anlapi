//go:build unit

package service

import (
	"context"
	"errors"
	"net/http"
	"testing"

	infraerrors "anlapi/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

type sparkShadowRepoStub struct {
	mockAccountRepoForGemini
	nextID   int64
	accounts map[int64]*Account
	groupsOf map[int64][]int64
	bindErr  error
}

func newSparkShadowRepoStub() *sparkShadowRepoStub {
	return &sparkShadowRepoStub{
		accounts: make(map[int64]*Account),
		groupsOf: make(map[int64][]int64),
		mockAccountRepoForGemini: mockAccountRepoForGemini{
			accountsByID: make(map[int64]*Account),
		},
	}
}

func (s *sparkShadowRepoStub) Create(_ context.Context, account *Account) error {
	s.nextID++
	account.ID = s.nextID
	stored := *account
	s.accounts[account.ID] = &stored
	s.accountsByID[account.ID] = &stored
	return nil
}

func (s *sparkShadowRepoStub) GetByID(_ context.Context, id int64) (*Account, error) {
	account, ok := s.accounts[id]
	if !ok {
		return nil, ErrAccountNotFound
	}
	return account, nil
}

func (s *sparkShadowRepoStub) ListShadowsByParent(_ context.Context, parentID int64) ([]*Account, error) {
	result := make([]*Account, 0, 1)
	for _, account := range s.accounts {
		if account.ParentAccountID != nil && *account.ParentAccountID == parentID && account.QuotaDimension == QuotaDimensionSpark {
			stored := *account
			result = append(result, &stored)
		}
	}
	return result, nil
}

func (s *sparkShadowRepoStub) BindGroups(_ context.Context, accountID int64, groupIDs []int64) error {
	if s.bindErr != nil {
		return s.bindErr
	}
	s.groupsOf[accountID] = append([]int64(nil), groupIDs...)
	return nil
}

func (s *sparkShadowRepoStub) Update(_ context.Context, account *Account) error {
	if _, ok := s.accounts[account.ID]; !ok {
		return ErrAccountNotFound
	}
	stored := *account
	s.accounts[account.ID] = &stored
	s.accountsByID[account.ID] = &stored
	return nil
}

func (s *sparkShadowRepoStub) Delete(_ context.Context, id int64) error {
	delete(s.accounts, id)
	delete(s.accountsByID, id)
	delete(s.groupsOf, id)
	return nil
}

func TestCreateShadowCreatesLinkedSparkAccount(t *testing.T) {
	ctx := context.Background()
	repo := newSparkShadowRepoStub()
	proxyID := int64(17)
	parent := &Account{
		Name:        "parent",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Credentials: map[string]any{"access_token": "parent-token"},
		ProxyID:     &proxyID,
		Priority:    40,
		Concurrency: 3,
		GroupIDs:    []int64{7, 8},
		Extra:       map[string]any{openAILongContextBillingEnabledKey: true},
	}
	require.NoError(t, repo.Create(ctx, parent))
	svc := &adminServiceImpl{accountRepo: repo}

	shadow, err := svc.CreateShadow(ctx, parent.ID, ShadowOptions{})

	require.NoError(t, err)
	require.Equal(t, "parent (Spark)", shadow.Name)
	require.Equal(t, parent.ID, *shadow.ParentAccountID)
	require.Equal(t, QuotaDimensionSpark, shadow.QuotaDimension)
	require.Equal(t, parent.ProxyID, shadow.ProxyID)
	require.Equal(t, parent.Priority, shadow.Priority)
	require.Equal(t, parent.Concurrency, shadow.Concurrency)
	require.Equal(t, parent.GroupIDs, shadow.GroupIDs)
	require.Equal(t, parent.GroupIDs, repo.groupsOf[shadow.ID])
	require.True(t, shadow.IsOpenAILongContextBillingEnabled())
	require.NotContains(t, shadow.Credentials, "access_token")
	require.Equal(t, defaultSparkShadowModelMapping(), shadow.Credentials["model_mapping"])

	_, err = svc.CreateShadow(ctx, parent.ID, ShadowOptions{Name: "duplicate"})
	require.Error(t, err)
	require.Equal(t, http.StatusConflict, infraerrors.Code(err))
	require.Equal(t, "SPARK_SHADOW_ALREADY_EXISTS", infraerrors.Reason(err))
}

func TestCreateShadowRejectsInvalidParentAndRollsBackBindFailure(t *testing.T) {
	ctx := context.Background()
	t.Run("invalid parent", func(t *testing.T) {
		repo := newSparkShadowRepoStub()
		parent := &Account{Name: "api-key", Platform: PlatformOpenAI, Type: AccountTypeAPIKey}
		require.NoError(t, repo.Create(ctx, parent))
		_, err := (&adminServiceImpl{accountRepo: repo}).CreateShadow(ctx, parent.ID, ShadowOptions{})
		require.Error(t, err)
		require.Equal(t, "SPARK_SHADOW_INVALID_PARENT", infraerrors.Reason(err))
	})

	t.Run("bind rollback", func(t *testing.T) {
		repo := newSparkShadowRepoStub()
		parent := &Account{Name: "parent", Platform: PlatformOpenAI, Type: AccountTypeOAuth, GroupIDs: []int64{9}}
		require.NoError(t, repo.Create(ctx, parent))
		repo.bindErr = errors.New("bind failed")
		_, err := (&adminServiceImpl{accountRepo: repo}).CreateShadow(ctx, parent.ID, ShadowOptions{})
		require.Error(t, err)
		shadows, listErr := repo.ListShadowsByParent(ctx, parent.ID)
		require.NoError(t, listErr)
		require.Empty(t, shadows)
	})
}

func TestSparkShadowProxyCredentialsAndDeleteInvariants(t *testing.T) {
	ctx := context.Background()
	repo := newSparkShadowRepoStub()
	parent := &Account{Name: "parent", Platform: PlatformOpenAI, Type: AccountTypeOAuth, Priority: 50, Concurrency: 2}
	require.NoError(t, repo.Create(ctx, parent))
	svc := &adminServiceImpl{accountRepo: repo}
	shadow, err := svc.CreateShadow(ctx, parent.ID, ShadowOptions{Name: "shadow"})
	require.NoError(t, err)

	proxyID := int64(23)
	_, err = svc.UpdateAccount(ctx, parent.ID, &UpdateAccountInput{ProxyID: &proxyID})
	require.NoError(t, err)
	updatedShadow, err := repo.GetByID(ctx, shadow.ID)
	require.NoError(t, err)
	require.Equal(t, &proxyID, updatedShadow.ProxyID)

	_, err = svc.UpdateAccount(ctx, shadow.ID, &UpdateAccountInput{Credentials: map[string]any{"access_token": "forbidden"}})
	require.Error(t, err)
	require.Equal(t, "SPARK_SHADOW_NO_CREDENTIALS", infraerrors.Reason(err))

	require.NoError(t, svc.DeleteAccount(ctx, parent.ID))
	_, parentErr := repo.GetByID(ctx, parent.ID)
	_, shadowErr := repo.GetByID(ctx, shadow.ID)
	require.Error(t, parentErr)
	require.Error(t, shadowErr)
}
