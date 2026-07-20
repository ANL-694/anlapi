package repository

import (
	"context"
	"fmt"
	"testing"

	"anl-api/internal/service"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

type migrationTestVault struct {
	entries map[string]map[string]any
	next    int
}

func (v *migrationTestVault) Mode() service.OAuthCredentialVaultMode {
	return service.OAuthCredentialVaultModeExternal
}

func (v *migrationTestVault) LegacyFallbackEnabled() bool { return true }

func (v *migrationTestVault) Get(_ context.Context, key service.OAuthCredentialVaultKey, version string) (map[string]any, error) {
	payload, ok := v.entries[migrationTestVaultEntryKey(key, version)]
	if !ok {
		return nil, service.ErrOAuthCredentialVaultEntryNotFound
	}
	return payload, nil
}

func (v *migrationTestVault) Put(_ context.Context, key service.OAuthCredentialVaultKey, payload map[string]any) (string, error) {
	if v.entries == nil {
		v.entries = make(map[string]map[string]any)
	}
	v.next++
	version := fmt.Sprintf("v%d", v.next)
	v.entries[migrationTestVaultEntryKey(key, version)] = payload
	return version, nil
}

func (v *migrationTestVault) Delete(context.Context, service.OAuthCredentialVaultKey) error {
	return nil
}

func (v *migrationTestVault) Close() error { return nil }

func migrationTestVaultEntryKey(key service.OAuthCredentialVaultKey, version string) string {
	ownerID := int64(0)
	if key.OwnerUserID != nil {
		ownerID = *key.OwnerUserID
	}
	return fmt.Sprintf("%d:%d:%s", key.AccountID, ownerID, version)
}

func TestMigrateOAuthCredentialsToVaultSanitizesAndVerifies(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	vault := &migrationTestVault{}
	raw := `{"access_token":"secret","chatgpt_account_id":"acct"}`
	sanitized := `{"_oauth_vault":{"version":"v1"},"chatgpt_account_id":"acct"}`

	mock.ExpectBegin()
	mock.ExpectExec(`LOCK TABLE accounts`).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(`SELECT id, owner_user_id, credentials`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "owner_user_id", "credentials"}).AddRow(int64(7), nil, []byte(raw)))
	mock.ExpectExec(`UPDATE accounts`).
		WithArgs(sanitized, int64(7), nil, raw).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	mock.ExpectQuery(`SELECT id, owner_user_id, credentials`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "owner_user_id", "credentials"}).AddRow(int64(7), nil, []byte(sanitized)))

	report, err := MigrateOAuthCredentialsToVault(context.Background(), db, vault)

	require.NoError(t, err)
	require.Equal(t, 1, report.Accounts)
	require.Equal(t, 1, report.MigratedRows)
	require.Zero(t, report.SensitiveRows)
	require.Equal(t, 1, report.MarkerRows)
	require.Zero(t, report.MissingVaultEntries)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAuditOAuthCredentialIsolationReportsSensitiveRows(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	vault := &migrationTestVault{}

	mock.ExpectQuery(`SELECT id, owner_user_id, credentials`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "owner_user_id", "credentials"}).
			AddRow(int64(9), int64(3), []byte(`{"refresh_token":"secret","email":"user@example.test"}`)))

	report, err := AuditOAuthCredentialIsolation(context.Background(), db, vault)

	require.NoError(t, err)
	require.Equal(t, 1, report.Accounts)
	require.Equal(t, 1, report.SensitiveRows)
	require.Zero(t, report.MarkerRows)
	require.NoError(t, mock.ExpectationsWereMet())
}
