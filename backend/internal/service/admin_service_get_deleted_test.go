package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type getDeletedUserRepoStub struct {
	UserRepository
	user  *User
	gotID int64
}

func (s *getDeletedUserRepoStub) GetByIDIncludeDeleted(_ context.Context, id int64) (*User, error) {
	s.gotID = id
	return s.user, nil
}

func TestAdminServiceGetUserIncludeDeleted(t *testing.T) {
	deletedAt := time.Date(2026, 5, 28, 0, 0, 0, 0, time.UTC)
	repo := &getDeletedUserRepoStub{
		user: &User{ID: 7, Email: "deleted@test.com", DeletedAt: &deletedAt},
	}
	adminService := &adminServiceImpl{userRepo: repo}

	user, err := adminService.GetUserIncludeDeleted(context.Background(), 7)
	require.NoError(t, err)
	require.Equal(t, int64(7), repo.gotID)
	require.Equal(t, int64(7), user.ID)
	require.NotNil(t, user.DeletedAt)
}
