package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/apikey"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

const systemAPIKeyBindingsTable = "system_api_key_bindings"

// EnsureSystemAPIKey creates or returns the single durable key for a
// user/purpose pair. The unique binding constraint is the cross-instance
// winner election; unused losers are tombstoned in the same transaction.
func (r *apiKeyRepository) EnsureSystemAPIKey(ctx context.Context, candidate *service.APIKey, purpose string) (*service.APIKey, error) {
	if candidate == nil || candidate.UserID <= 0 || strings.TrimSpace(candidate.Key) == "" || strings.TrimSpace(purpose) == "" {
		return nil, fmt.Errorf("invalid system api key candidate")
	}

	tx, err := r.client.Tx(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	txClient := tx.Client()
	executor, ok := txClient.Driver().(sqlExecutor)
	if !ok {
		return nil, service.ErrSystemAPIKeyStoreUnavailable
	}

	boundID, bound, err := querySystemAPIKeyBindingID(ctx, executor, candidate.UserID, purpose)
	if err != nil {
		return nil, wrapSystemAPIKeyStoreError(err)
	}
	if bound {
		existing, loadErr := loadActiveSystemAPIKey(ctx, txClient, candidate.UserID, boundID, purpose)
		if loadErr == nil {
			if err := tx.Commit(); err != nil {
				return nil, err
			}
			return existing, nil
		}
		if !errors.Is(loadErr, service.ErrAPIKeyNotFound) {
			return nil, loadErr
		}
	}

	if err := createSystemAPIKeyInTx(ctx, txClient, candidate); err != nil {
		return nil, err
	}

	wonBinding := false
	if bound {
		result, updateErr := executor.ExecContext(ctx, `
			UPDATE system_api_key_bindings
			SET api_key_id = $1, updated_at = CURRENT_TIMESTAMP
			WHERE user_id = $2 AND purpose = $3 AND api_key_id = $4`,
			candidate.ID, candidate.UserID, purpose, boundID)
		if updateErr != nil {
			return nil, wrapSystemAPIKeyStoreError(updateErr)
		}
		affected, affectedErr := result.RowsAffected()
		if affectedErr != nil {
			return nil, affectedErr
		}
		wonBinding = affected == 1
	} else {
		result, insertErr := executor.ExecContext(ctx, `
			INSERT INTO system_api_key_bindings (user_id, api_key_id, purpose)
			VALUES ($1, $2, $3)
			ON CONFLICT (user_id, purpose) DO NOTHING`,
			candidate.UserID, candidate.ID, purpose)
		if insertErr != nil {
			return nil, wrapSystemAPIKeyStoreError(insertErr)
		}
		affected, affectedErr := result.RowsAffected()
		if affectedErr != nil {
			return nil, affectedErr
		}
		wonBinding = affected == 1
	}

	if wonBinding {
		candidate.ManagedType = purpose
		if err := tx.Commit(); err != nil {
			return nil, err
		}
		return candidate, nil
	}

	if err := softDeleteSystemAPIKeyCandidate(ctx, txClient, candidate.ID); err != nil {
		return nil, err
	}
	winnerID, found, err := querySystemAPIKeyBindingID(ctx, executor, candidate.UserID, purpose)
	if err != nil {
		return nil, wrapSystemAPIKeyStoreError(err)
	}
	if !found {
		return nil, fmt.Errorf("system api key binding disappeared during reconciliation")
	}
	winner, err := loadActiveSystemAPIKey(ctx, txClient, candidate.UserID, winnerID, purpose)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return winner, nil
}

func createSystemAPIKeyInTx(ctx context.Context, client *dbent.Client, key *service.APIKey) error {
	builder := client.APIKey.Create().
		SetUserID(key.UserID).
		SetKey(key.Key).
		SetName(key.Name).
		SetStatus(key.Status).
		SetNillableGroupID(key.GroupID).
		SetNillableLastUsedAt(key.LastUsedAt).
		SetQuota(key.Quota).
		SetQuotaUsed(key.QuotaUsed).
		SetNillableExpiresAt(key.ExpiresAt).
		SetRateLimit5h(key.RateLimit5h).
		SetRateLimit1d(key.RateLimit1d).
		SetRateLimit7d(key.RateLimit7d)
	if len(key.IPWhitelist) > 0 {
		builder.SetIPWhitelist(key.IPWhitelist)
	}
	if len(key.IPBlacklist) > 0 {
		builder.SetIPBlacklist(key.IPBlacklist)
	}
	created, err := builder.Save(ctx)
	if err != nil {
		return translatePersistenceError(err, nil, service.ErrAPIKeyExists)
	}
	key.ID = created.ID
	key.LastUsedAt = created.LastUsedAt
	key.CreatedAt = created.CreatedAt
	key.UpdatedAt = created.UpdatedAt
	return nil
}

func (r *apiKeyRepository) ListSystemAPIKeyPurposes(ctx context.Context, userID int64) (map[int64]string, error) {
	if r.sql == nil {
		return nil, service.ErrSystemAPIKeyStoreUnavailable
	}
	rows, err := r.sql.QueryContext(ctx, `
		SELECT binding.api_key_id, binding.purpose
		FROM system_api_key_bindings AS binding
		JOIN api_keys AS api_key ON api_key.id = binding.api_key_id
		WHERE binding.user_id = $1
		  AND api_key.user_id = binding.user_id
		  AND api_key.deleted_at IS NULL`, userID)
	if err != nil {
		return nil, wrapSystemAPIKeyStoreError(err)
	}
	defer func() { _ = rows.Close() }()
	purposes := make(map[int64]string)
	for rows.Next() {
		var keyID int64
		var purpose string
		if err := rows.Scan(&keyID, &purpose); err != nil {
			return nil, err
		}
		purposes[keyID] = purpose
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return purposes, nil
}

func (r *apiKeyRepository) GetSystemAPIKeyID(ctx context.Context, userID int64, purpose string) (int64, bool, error) {
	if r.sql == nil {
		return 0, false, service.ErrSystemAPIKeyStoreUnavailable
	}
	keyID, found, err := querySingleSystemAPIKeyID(ctx, r.sql, `
		SELECT binding.api_key_id
		FROM system_api_key_bindings AS binding
		JOIN api_keys AS api_key ON api_key.id = binding.api_key_id
		WHERE binding.user_id = $1
		  AND binding.purpose = $2
		  AND api_key.user_id = binding.user_id
		  AND api_key.deleted_at IS NULL
		LIMIT 1`, userID, purpose)
	if err != nil {
		return 0, false, wrapSystemAPIKeyStoreError(err)
	}
	return keyID, found, nil
}

func (r *apiKeyRepository) GetSystemAPIKeyPurpose(ctx context.Context, userID, keyID int64) (string, bool, error) {
	if r.sql == nil {
		return "", false, service.ErrSystemAPIKeyStoreUnavailable
	}
	value, found, err := querySingleSystemAPIKeyPurpose(ctx, r.sql, `
		SELECT binding.purpose
		FROM system_api_key_bindings AS binding
		JOIN api_keys AS api_key ON api_key.id = binding.api_key_id
		WHERE binding.user_id = $1
		  AND binding.api_key_id = $2
		  AND api_key.user_id = binding.user_id
		  AND api_key.deleted_at IS NULL
			LIMIT 1`, userID, keyID)
	if err != nil {
		return "", false, wrapSystemAPIKeyStoreError(err)
	}
	return value, found, nil
}

func querySystemAPIKeyBindingID(ctx context.Context, executor sqlExecutor, userID int64, purpose string) (int64, bool, error) {
	return querySingleSystemAPIKeyID(ctx, executor, `
		SELECT api_key_id
		FROM system_api_key_bindings
		WHERE user_id = $1 AND purpose = $2
		LIMIT 1`, userID, purpose)
}

func querySingleSystemAPIKeyID(ctx context.Context, executor sqlExecutor, query string, args ...any) (int64, bool, error) {
	rows, err := executor.QueryContext(ctx, query, args...)
	if err != nil {
		return 0, false, err
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		return 0, false, rows.Err()
	}
	var keyID int64
	if err := rows.Scan(&keyID); err != nil {
		return 0, false, err
	}
	return keyID, true, rows.Err()
}

func querySingleSystemAPIKeyPurpose(ctx context.Context, executor sqlExecutor, query string, args ...any) (string, bool, error) {
	rows, err := executor.QueryContext(ctx, query, args...)
	if err != nil {
		return "", false, err
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		return "", false, rows.Err()
	}
	var purpose string
	if err := rows.Scan(&purpose); err != nil {
		return "", false, err
	}
	return purpose, true, rows.Err()
}

func loadActiveSystemAPIKey(ctx context.Context, client *dbent.Client, userID, keyID int64, purpose string) (*service.APIKey, error) {
	entity, err := client.APIKey.Query().
		Where(apikey.IDEQ(keyID), apikey.UserIDEQ(userID), apikey.DeletedAtIsNil()).
		WithUser().
		WithGroup().
		Only(ctx)
	if err != nil {
		if dbent.IsNotFound(err) {
			return nil, service.ErrAPIKeyNotFound
		}
		return nil, err
	}
	key := apiKeyEntityToService(entity)
	key.ManagedType = purpose
	return key, nil
}

func softDeleteSystemAPIKeyCandidate(ctx context.Context, client *dbent.Client, keyID int64) error {
	tombstoneKey := fmt.Sprintf("__deleted_system__%d__%d", keyID, time.Now().UnixNano())
	result, err := client.ExecContext(ctx, `
		UPDATE api_keys
		SET key = $1, deleted_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP
		WHERE id = $2 AND deleted_at IS NULL`, tombstoneKey, keyID)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected != 1 {
		return service.ErrAPIKeyNotFound
	}
	return nil
}

func wrapSystemAPIKeyStoreError(err error) error {
	if err == nil {
		return nil
	}
	message := strings.ToLower(err.Error())
	if strings.Contains(message, systemAPIKeyBindingsTable) &&
		(strings.Contains(message, "does not exist") || strings.Contains(message, "no such table") || strings.Contains(message, "undefined table")) {
		return fmt.Errorf("%w: %v", service.ErrSystemAPIKeyStoreUnavailable, err)
	}
	return err
}

var _ service.SystemAPIKeyRepository = (*apiKeyRepository)(nil)
