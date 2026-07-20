package repository

import (
	"context"
	"testing"

	"anl-api/internal/config"
	"anl-api/internal/service"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

func disabledOAuthVault() service.OAuthCredentialVault {
	return &noopOAuthCredentialVault{mode: service.OAuthCredentialVaultModeDisabled}
}

func TestNewOAuthCredentialVaultNormalizesDisabledMode(t *testing.T) {
	vault, err := NewOAuthCredentialVault(&config.Config{
		OAuthVault: config.OAuthVaultConfig{Mode: " DISABLED "},
	})

	require.NoError(t, err)
	require.Equal(t, service.OAuthCredentialVaultModeDisabled, vault.Mode())
}

func TestPrepareOAuthCredentialsForWriteRejectsOAuthOnDisabledNode(t *testing.T) {
	repo := &accountRepository{oauthVault: disabledOAuthVault()}

	_, err := repo.prepareOAuthCredentialsForWrite(
		context.Background(),
		1,
		nil,
		service.AccountTypeOAuth,
		map[string]any{"access_token": "secret"},
	)

	require.ErrorIs(t, err, service.ErrOAuthCredentialVaultDisabled)
}

func TestCreateAndUpdateRejectOAuthOnDisabledNodeBeforeDatabaseWrite(t *testing.T) {
	repo := &accountRepository{oauthVault: disabledOAuthVault()}
	account := &service.Account{
		ID:          1,
		Type:        service.AccountTypeOAuth,
		Credentials: map[string]any{"refresh_token": "secret"},
	}

	require.ErrorIs(t, repo.createAccountRecord(context.Background(), nil, account), service.ErrOAuthCredentialVaultDisabled)
	require.ErrorIs(t, repo.Update(context.Background(), account), service.ErrOAuthCredentialVaultDisabled)
}

func TestRejectDisabledOAuthMutationChecksAllTargetAccounts(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	repo := &accountRepository{sql: db, oauthVault: disabledOAuthVault()}
	mock.ExpectQuery(`SELECT EXISTS`).
		WithArgs(pq.Array([]int64{1, 2})).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))

	err = repo.rejectDisabledOAuthMutation(context.Background(), []int64{1, 2})

	require.ErrorIs(t, err, service.ErrOAuthCredentialVaultDisabled)
	require.NoError(t, mock.ExpectationsWereMet())
}
