package repository

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

type testOAuthCredentialVault struct {
	mode      service.OAuthCredentialVaultMode
	fallback  bool
	payload   map[string]any
	getErr    error
	putErr    error
	lastKey   service.OAuthCredentialVaultKey
	activated string
}

func (v *testOAuthCredentialVault) ActivateVersion(_ context.Context, key service.OAuthCredentialVaultKey, version string) error {
	v.lastKey = key
	v.activated = version
	return nil
}

func (v *testOAuthCredentialVault) Mode() service.OAuthCredentialVaultMode { return v.mode }
func (v *testOAuthCredentialVault) LegacyFallbackEnabled() bool            { return v.fallback }
func (v *testOAuthCredentialVault) Get(_ context.Context, key service.OAuthCredentialVaultKey, _ string) (map[string]any, error) {
	v.lastKey = key
	if v.getErr != nil {
		return nil, v.getErr
	}
	return service.MergeOAuthCredentials(nil, v.payload), nil
}
func (v *testOAuthCredentialVault) Put(_ context.Context, key service.OAuthCredentialVaultKey, payload map[string]any) (string, error) {
	v.lastKey = key
	if v.putErr != nil {
		return "", v.putErr
	}
	v.payload = service.MergeOAuthCredentials(nil, payload)
	return "test-version", nil
}
func (v *testOAuthCredentialVault) Delete(context.Context, service.OAuthCredentialVaultKey) error {
	return nil
}
func (v *testOAuthCredentialVault) Close() error { return nil }

func TestOAuthCredentialVaultAESGCMBindsAccountID(t *testing.T) {
	vault := &oauthCredentialVault{key: make([]byte, 32)}
	key := service.OAuthCredentialVaultKey{AccountID: 41}
	ciphertext, err := vault.encrypt([]byte(`{"refresh_token":"test-token"}`), key)
	require.NoError(t, err)

	plaintext, err := vault.decrypt(ciphertext, key)
	require.NoError(t, err)
	require.JSONEq(t, `{"refresh_token":"test-token"}`, string(plaintext))

	_, err = vault.decrypt(ciphertext, service.OAuthCredentialVaultKey{AccountID: 42})
	require.Error(t, err)
}

func TestOAuthCredentialVaultDatabaseIdentityRejectsAliasesForSameDatabase(t *testing.T) {
	require.True(t, samePostgresDatabase(
		postgresDatabaseIdentity{SystemID: "cluster-a", Address: "127.0.0.1", Port: 5432, Database: "sub2api"},
		postgresDatabaseIdentity{SystemID: "cluster-a", Address: "10.0.0.9", Port: 6432, Database: "sub2api"},
	))
	require.False(t, samePostgresDatabase(
		postgresDatabaseIdentity{Address: "127.0.0.1", Port: 5432, Database: "sub2api"},
		postgresDatabaseIdentity{Address: "127.0.0.1", Port: 5432, Database: "sub2api"},
	))
	require.False(t, samePostgresDatabase(
		postgresDatabaseIdentity{SystemID: "cluster-a", Address: "127.0.0.1", Port: 5432, Database: "sub2api_vault"},
		postgresDatabaseIdentity{SystemID: "cluster-a", Address: "127.0.0.1", Port: 5432, Database: "sub2api"},
	))
}

func TestLoadPostgresDatabaseIdentityFailsClosedWithoutSystemIdentifier(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	mock.ExpectQuery("SELECT COALESCE").WillReturnRows(
		sqlmock.NewRows([]string{"address", "port", "database"}).AddRow("127.0.0.1", 5432, "sub2api"),
	)
	mock.ExpectQuery("SELECT system_identifier").WillReturnError(errors.New("permission denied"))

	_, err = loadPostgresDatabaseIdentity(context.Background(), db)
	require.ErrorContains(t, err, "system identifier")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestOAuthCredentialVaultCleanupStartsGraceBeforeDeleting(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	vault := &oauthCredentialVault{db: db}
	cutoff := time.Now().Add(-15 * time.Minute)

	mock.ExpectQuery("SELECT account_id, version, unreferenced_since, state").
		WillReturnRows(sqlmock.NewRows([]string{"account_id", "version", "unreferenced_since", "state"}).AddRow(int64(41), "old", nil, "active"))
	mock.ExpectExec("UPDATE oauth_credential_vault").
		WithArgs(int64(41), "old").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT account_id").
		WithArgs(cutoff).
		WillReturnRows(sqlmock.NewRows([]string{"account_id"}))
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM oauth_credential_vault_delete_pending`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	report, err := vault.CleanupUnreferenced(context.Background(), map[int64]string{41: "current"}, cutoff)
	require.NoError(t, err)
	require.Zero(t, report.ReclaimedVersions)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestOAuthCredentialVaultCleanupDeletesOnlyAfterGrace(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	vault := &oauthCredentialVault{db: db}
	cutoff := time.Now().Add(-15 * time.Minute)

	mock.ExpectQuery("SELECT account_id, version, unreferenced_since, state").
		WillReturnRows(sqlmock.NewRows([]string{"account_id", "version", "unreferenced_since", "state"}).AddRow(int64(41), "old", cutoff.Add(-time.Minute), "active"))
	mock.ExpectExec("DELETE FROM oauth_credential_vault").
		WithArgs(int64(41), "old", cutoff).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT account_id").
		WithArgs(cutoff).
		WillReturnRows(sqlmock.NewRows([]string{"account_id"}))
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM oauth_credential_vault_delete_pending`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	report, err := vault.CleanupUnreferenced(context.Background(), map[int64]string{41: "current"}, cutoff)
	require.NoError(t, err)
	require.Equal(t, 1, report.ReclaimedVersions)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestOAuthCredentialVaultCleanupNeverDeletesPendingWriteAndActivatesReferencedPending(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	vault := &oauthCredentialVault{db: db}
	cutoff := time.Now().Add(-15 * time.Minute)

	mock.ExpectQuery("SELECT account_id, version, unreferenced_since, state").WillReturnRows(
		sqlmock.NewRows([]string{"account_id", "version", "unreferenced_since", "state"}).
			AddRow(int64(41), "in-flight", cutoff.Add(-time.Hour), "pending").
			AddRow(int64(42), "committed", nil, "pending"),
	)
	mock.ExpectExec("UPDATE oauth_credential_vault").WithArgs(int64(42), "committed").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT account_id").WithArgs(cutoff).WillReturnRows(sqlmock.NewRows([]string{"account_id"}))
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM oauth_credential_vault_delete_pending`).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	report, err := vault.CleanupUnreferenced(context.Background(), map[int64]string{42: "committed"}, cutoff)
	require.NoError(t, err)
	require.Zero(t, report.ReclaimedVersions)
	require.Equal(t, 1, report.PendingWrites)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestOAuthVaultRecoveryPreservesUnknownBusinessCommit(t *testing.T) {
	require.True(t, shouldDiscardOAuthVaultWrite(false, false))
	require.False(t, shouldDiscardOAuthVaultWrite(true, true))
	require.False(t, shouldDiscardOAuthVaultWrite(false, true), "commit acknowledgement loss must preserve the pending version")

	require.True(t, shouldCancelOAuthVaultDelete(true, false))
	require.False(t, shouldCancelOAuthVaultDelete(false, true))
	require.False(t, shouldCancelOAuthVaultDelete(true, true), "commit acknowledgement loss must preserve the delete tombstone")
}

func TestOAuthVaultRecoveryContextIsBoundedAndDetachedFromCancellation(t *testing.T) {
	parent, cancelParent := context.WithCancel(context.Background())
	cancelParent()
	recoveryCtx, cancel := oauthVaultRecoveryContext(parent)
	defer cancel()

	require.NoError(t, recoveryCtx.Err())
	deadline, ok := recoveryCtx.Deadline()
	require.True(t, ok)
	require.WithinDuration(t, time.Now().Add(2*time.Second), deadline, 250*time.Millisecond)
}

func TestAccountRepositoryExternalVaultHydratesAndFailsClosed(t *testing.T) {
	vault := &testOAuthCredentialVault{
		mode:    service.OAuthCredentialVaultModeExternal,
		payload: map[string]any{"refresh_token": "test-refresh"},
	}
	repo := &accountRepository{oauthVault: vault}
	account := &service.Account{
		ID:          41,
		Type:        service.AccountTypeOAuth,
		Credentials: service.SetOAuthCredentialVaultVersion(map[string]any{"base_url": "https://example.test"}, "v1"),
	}

	require.NoError(t, repo.hydrateOAuthCredentials(context.Background(), account))
	require.Equal(t, int64(41), vault.lastKey.AccountID)
	require.Equal(t, "test-refresh", account.Credentials["refresh_token"])
	require.Equal(t, "https://example.test", account.Credentials["base_url"])

	account.Credentials = map[string]any{"base_url": "https://example.test"}
	err := repo.hydrateOAuthCredentials(context.Background(), account)
	require.ErrorContains(t, err, "version missing")

	vault.getErr = service.ErrOAuthCredentialVaultEntryNotFound
	account.Credentials = service.SetOAuthCredentialVaultVersion(map[string]any{}, "v1")
	err = repo.hydrateOAuthCredentials(context.Background(), account)
	require.ErrorIs(t, err, service.ErrOAuthCredentialVaultEntryNotFound)
}

func TestAccountRepositoryExternalVaultSanitizesWritesAndRestoresCaller(t *testing.T) {
	vault := &testOAuthCredentialVault{mode: service.OAuthCredentialVaultModeExternal}
	repo := &accountRepository{oauthVault: vault}
	account := &service.Account{
		ID:   72,
		Type: service.AccountTypeOAuth,
		Credentials: map[string]any{
			"base_url":      "https://example.test",
			"refresh_token": "test-refresh",
		},
	}

	restore, _, err := repo.prepareOAuthCredentialsForWrite(context.Background(), account)
	require.NoError(t, err)
	require.Equal(t, int64(72), vault.lastKey.AccountID)
	require.Equal(t, "test-refresh", vault.payload["refresh_token"])
	require.NotContains(t, account.Credentials, "refresh_token")
	require.Equal(t, "test-version", service.OAuthCredentialVaultVersion(account.Credentials))

	restore()
	require.Equal(t, "test-refresh", account.Credentials["refresh_token"])

	vault.putErr = errors.New("vault unavailable")
	_, _, err = repo.prepareOAuthCredentialsForWrite(context.Background(), account)
	require.ErrorContains(t, err, "vault unavailable")
}

func TestAccountRepositoryVaultDisabledRejectsOAuthWritesAndExternalCASSanitizesTokens(t *testing.T) {
	disabled := &accountRepository{oauthVault: &testOAuthCredentialVault{mode: service.OAuthCredentialVaultModeDisabled}}
	_, _, err := disabled.prepareOAuthCredentialsForWrite(context.Background(), &service.Account{
		ID: 1, Type: service.AccountTypeOAuth, Credentials: map[string]any{"refresh_token": "test-refresh"},
	})
	require.ErrorIs(t, err, service.ErrOAuthCredentialVaultDisabled)

	external := &accountRepository{oauthVault: &testOAuthCredentialVault{mode: service.OAuthCredentialVaultModeExternal}}
	stored, err := external.oauthCredentialsForCAS(service.MergeOAuthCredentials(
		service.SetOAuthCredentialVaultVersion(map[string]any{"base_url": "https://example.test"}, "v1"),
		map[string]any{"refresh_token": "test-refresh"},
	))
	require.NoError(t, err)
	require.NotContains(t, stored, "refresh_token")
	require.Equal(t, "v1", service.OAuthCredentialVaultVersion(stored))
}

func TestAccountRepositoryVaultDisabledRejectsGroupedOAuthCreateBeforeDatabaseUse(t *testing.T) {
	repo := &accountRepository{oauthVault: &testOAuthCredentialVault{mode: service.OAuthCredentialVaultModeDisabled}}
	err := repo.CreateWithAccountGroups(context.Background(), &service.Account{Type: service.AccountTypeOAuth}, nil)
	require.ErrorIs(t, err, service.ErrOAuthCredentialVaultDisabled)
}

func TestAccountRepositoryExternalVaultRejectsOAuthTypeEscape(t *testing.T) {
	repo := &accountRepository{oauthVault: &testOAuthCredentialVault{mode: service.OAuthCredentialVaultModeExternal}}
	err := repo.updateAccount(context.Background(), &service.Account{
		ID:   72,
		Type: service.AccountTypeAPIKey,
		Credentials: service.SetOAuthCredentialVaultVersion(
			map[string]any{"refresh_token": "must-not-replicate"},
			"vault-version",
		),
	}, nil, nil, nil)
	require.ErrorContains(t, err, "non-OAuth type")
}

func TestValidateOAuthVaultTypeTransitionUsesPersistedType(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	mock.ExpectQuery("SELECT type").WithArgs(int64(72)).WillReturnRows(
		sqlmock.NewRows([]string{"type"}).AddRow(service.AccountTypeOAuth),
	)
	err = validateOAuthVaultTypeTransition(
		context.Background(),
		db,
		&testOAuthCredentialVault{mode: service.OAuthCredentialVaultModeExternal},
		72,
		service.AccountTypeAPIKey,
	)
	require.ErrorContains(t, err, "OAuth Vault boundary")
	require.NoError(t, mock.ExpectationsWereMet())
}
