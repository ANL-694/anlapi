package service

import (
	"context"
	"testing"

	"anlapi/internal/pkg/ctxkey"
	"github.com/stretchr/testify/require"
)

func TestIsAccountVisibleToRequestUserAllowsOwnerPublicShareModeInPrivateGroup(t *testing.T) {
	ownerID := int64(42)
	account := &Account{
		ID:          10,
		OwnerUserID: &ownerID,
		ShareMode:   AccountShareModePublic,
		ShareStatus: AccountShareStatusPending,
	}
	ctx := context.WithValue(context.Background(), ctxkey.AuthenticatedUserID, ownerID)
	ctx = context.WithValue(ctx, ctxkey.Group, &Group{
		ID:          99,
		Scope:       GroupScopeUserPrivate,
		OwnerUserID: &ownerID,
	})

	require.True(t, IsAccountVisibleToRequestUser(ctx, account))

	nonOwnerContext := context.WithValue(context.Background(), ctxkey.AuthenticatedUserID, int64(100))
	nonOwnerContext = context.WithValue(nonOwnerContext, ctxkey.Group, &Group{
		ID:          99,
		Scope:       GroupScopeUserPrivate,
		OwnerUserID: &ownerID,
	})
	require.False(t, IsAccountVisibleToRequestUser(nonOwnerContext, account))

	account.ShareMode = AccountShareModePrivate
	require.True(t, IsAccountVisibleToRequestUser(ctx, account))
}

func TestIsAccountAllowedForRequestGroupRejectsPublicShareModeFromCarpoolGroup(t *testing.T) {
	ownerID := int64(42)
	account := &Account{OwnerUserID: &ownerID, ShareMode: AccountShareModePublic}
	ctx := context.WithValue(context.Background(), ctxkey.AuthenticatedUserID, ownerID)
	ctx = context.WithValue(ctx, ctxkey.Group, &Group{Scope: GroupScopeUserCarpool, OwnerUserID: &ownerID})

	require.False(t, IsAccountAllowedForRequestGroup(ctx, account))
}

func TestIsAccountVisibleToRequestUserKeepsApprovedPublicShareVisibleInPublicGroup(t *testing.T) {
	ownerID := int64(42)
	consumerID := int64(100)
	account := &Account{
		ID:          10,
		OwnerUserID: &ownerID,
		ShareMode:   AccountShareModePublic,
		ShareStatus: AccountShareStatusApproved,
	}
	ctx := context.WithValue(context.Background(), ctxkey.AuthenticatedUserID, consumerID)
	ctx = context.WithValue(ctx, ctxkey.Group, &Group{
		ID:    6,
		Scope: GroupScopePublic,
	})

	require.True(t, IsAccountVisibleToRequestUser(ctx, account))
}
