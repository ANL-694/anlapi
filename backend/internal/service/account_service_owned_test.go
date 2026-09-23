package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/stretchr/testify/require"
)

type ownedAccountServiceRepoStub struct {
	AccountRepository
	account       *Account
	accounts      []Account
	listOwnerID   int64
	updated       bool
	deleted       bool
	ownedUpdateID int64
	ownedDeleteID int64
	createdGroups []int64
}

func (r *ownedAccountServiceRepoStub) ListWithFilters(_ context.Context, params pagination.PaginationParams, _, _, _, _ string, _ int64, _ string) ([]Account, *pagination.PaginationResult, error) {
	return r.accounts, ownedPagination(int64(len(r.accounts)), params), nil
}

type ownedGroupRepoStub struct {
	GroupRepository
	groups map[int64]*Group
}

func (r *ownedGroupRepoStub) GetByID(_ context.Context, id int64) (*Group, error) {
	group := r.groups[id]
	if group == nil {
		return nil, ErrGroupNotFound
	}
	copy := *group
	return &copy, nil
}

func ownedPagination(total int64, params pagination.PaginationParams) *pagination.PaginationResult {
	page := params.Page
	if page < 1 {
		page = 1
	}
	pageSize := params.PageSize
	if pageSize < 1 {
		pageSize = 20
	}
	pages := int((total + int64(pageSize) - 1) / int64(pageSize))
	if pages < 1 {
		pages = 1
	}
	return &pagination.PaginationResult{Total: total, Page: page, PageSize: pageSize, Pages: pages}
}

func (r *ownedAccountServiceRepoStub) ListOwnedWithFilters(ctx context.Context, ownerUserID int64, params pagination.PaginationParams, platform, accountType, status, search string, groupID int64, privacyMode string) ([]Account, *pagination.PaginationResult, error) {
	r.listOwnerID = ownerUserID
	if r.account == nil || r.account.OwnerUserID == nil || *r.account.OwnerUserID != ownerUserID {
		return nil, ownedPagination(0, params), nil
	}
	return []Account{*r.account}, ownedPagination(1, params), nil
}

func (r *ownedAccountServiceRepoStub) GetOwnedByID(ctx context.Context, ownerUserID, accountID int64) (*Account, error) {
	if r.account == nil || r.account.ID != accountID || r.account.OwnerUserID == nil || *r.account.OwnerUserID != ownerUserID {
		return nil, ErrAccountNotFound
	}
	copy := *r.account
	return &copy, nil
}

func (r *ownedAccountServiceRepoStub) Create(ctx context.Context, account *Account) error {
	r.account = account
	r.account.ID = 101
	return nil
}

func (r *ownedAccountServiceRepoStub) Update(ctx context.Context, account *Account) error {
	r.updated = true
	r.account = account
	return nil
}

func (r *ownedAccountServiceRepoStub) UpdateOwned(_ context.Context, ownerUserID int64, account *Account) error {
	if account == nil || account.OwnerUserID == nil || *account.OwnerUserID != ownerUserID {
		return ErrAccountNotFound
	}
	r.ownedUpdateID = ownerUserID
	r.updated = true
	r.account = account
	return nil
}

func (r *ownedAccountServiceRepoStub) Delete(ctx context.Context, id int64) error {
	r.deleted = true
	return nil
}

func (r *ownedAccountServiceRepoStub) DeleteOwned(_ context.Context, ownerUserID, id int64) error {
	if r.account == nil || r.account.ID != id || r.account.OwnerUserID == nil || *r.account.OwnerUserID != ownerUserID {
		return ErrAccountNotFound
	}
	r.ownedDeleteID = ownerUserID
	r.deleted = true
	return nil
}

func (r *ownedAccountServiceRepoStub) BindGroups(ctx context.Context, accountID int64, groupIDs []int64) error {
	r.createdGroups = append([]int64(nil), groupIDs...)
	return nil
}

func TestAccountServiceOwnedCRUDUsesAuthenticatedOwner(t *testing.T) {
	ctx := context.Background()
	ownerID := int64(7)
	otherID := int64(8)
	repo := &ownedAccountServiceRepoStub{}
	svc := NewAccountService(repo, nil)

	created, err := svc.CreateOwned(ctx, ownerID, CreateAccountRequest{
		Name:        "personal",
		Platform:    PlatformAnthropic,
		Type:        AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "redacted-in-test"},
	})
	require.NoError(t, err)
	require.NotNil(t, created.OwnerUserID)
	require.Equal(t, ownerID, *created.OwnerUserID)

	items, result, err := svc.ListOwned(ctx, ownerID, pagination.PaginationParams{Page: 1, PageSize: 20}, AccountListFilters{})
	require.NoError(t, err)
	require.Equal(t, ownerID, repo.listOwnerID)
	require.Len(t, items, 1)
	require.EqualValues(t, 1, result.Total)

	_, err = svc.GetOwnedByID(ctx, otherID, created.ID)
	require.ErrorIs(t, err, ErrAccountNotFound)

	name := "renamed"
	updated, err := svc.UpdateOwned(ctx, ownerID, created.ID, UpdateAccountRequest{Name: &name})
	require.NoError(t, err)
	require.True(t, repo.updated)
	require.Equal(t, ownerID, repo.ownedUpdateID)
	require.Equal(t, name, updated.Name)

	require.NoError(t, svc.DeleteOwned(ctx, ownerID, created.ID))
	require.True(t, repo.deleted)
	require.Equal(t, ownerID, repo.ownedDeleteID)
}

func TestGetQuotaPoolDashboardSeparatesPrivateAndPublicAccounts(t *testing.T) {
	ownerID := int64(7)
	otherID := int64(8)
	sharedGroup := &Group{ID: 11, Name: "shared", Scope: GroupScopePublic, Status: StatusActive, SubscriptionType: SubscriptionTypeStandard}
	private := &Group{ID: 12, Name: "private", Scope: GroupScopeUserPrivate, OwnerUserID: &ownerID, Status: StatusActive}
	r := &ownedAccountServiceRepoStub{accounts: []Account{
		{ID: 1, OwnerUserID: &ownerID, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Groups: []*Group{private}},
		{ID: 2, OwnerUserID: &otherID, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Groups: []*Group{private}},
		{ID: 3, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Groups: []*Group{sharedGroup}},
		{ID: 4, OwnerUserID: &otherID, ShareMode: AccountShareModePublic, ShareStatus: AccountShareStatusPending, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Groups: []*Group{sharedGroup}},
	}}
	svc := NewAccountService(r, nil)
	dashboard, err := svc.GetQuotaPoolDashboard(context.Background(), ownerID)
	require.NoError(t, err)
	require.NotNil(t, dashboard)
	require.Equal(t, 1, dashboard.Mine.Totals.AccountCount)
	require.Equal(t, 1, dashboard.Platform.Totals.AccountCount)
}

func TestAccountServiceOwnedGroupBindingRejectsAnotherUsersPrivateGroup(t *testing.T) {
	ctx := context.Background()
	ownerID := int64(7)
	otherOwnerID := int64(8)
	groupRepo := &ownedGroupRepoStub{groups: map[int64]*Group{
		10: {ID: 10, Scope: GroupScopeUserPrivate, OwnerUserID: &otherOwnerID},
	}}
	svc := NewAccountService(&ownedAccountServiceRepoStub{}, groupRepo)

	_, err := svc.CreateOwned(ctx, ownerID, CreateAccountRequest{
		Name:        "personal",
		Platform:    PlatformAnthropic,
		Type:        AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "redacted-in-test"},
		GroupIDs:    []int64{10},
	})
	require.ErrorIs(t, err, ErrGroupNotFound)
}

func TestAccountServiceBulkOwnedOperationsReportPerAccountResults(t *testing.T) {
	ctx := context.Background()
	ownerID := int64(7)
	repo := &ownedAccountServiceRepoStub{}
	svc := NewAccountService(repo, nil)
	created, err := svc.CreateOwned(ctx, ownerID, CreateAccountRequest{
		Name:        "personal",
		Platform:    PlatformAnthropic,
		Type:        AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "redacted-in-test"},
	})
	require.NoError(t, err)

	enabled := false
	updated, err := svc.BulkUpdateOwned(ctx, ownerID, &BulkUpdateOwnedAccountsInput{
		AccountIDs:  []int64{created.ID, created.ID, 999},
		Schedulable: &enabled,
	})
	require.NoError(t, err)
	require.Equal(t, 1, updated.Success)
	require.Equal(t, 1, updated.Failed)
	require.Equal(t, []int64{created.ID}, updated.SuccessIDs)
	require.Equal(t, []int64{999}, updated.FailedIDs)

	deleted, err := svc.BulkDeleteOwned(ctx, ownerID, []int64{created.ID, 999})
	require.NoError(t, err)
	require.Equal(t, 1, deleted.Success)
	require.Equal(t, 1, deleted.Failed)
	require.True(t, repo.deleted)
}
