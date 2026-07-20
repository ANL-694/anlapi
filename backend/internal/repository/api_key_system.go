package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	dbent "anlapi/ent"
	"anlapi/ent/apikey"
	"anlapi/internal/service"
)

const systemAPIKeyBindingsTable = "system_api_key_bindings"

func (r *apiKeyRepository) EnsureSystemAPIKey(ctx context.Context, candidate *service.APIKey, purpose string) (*service.APIKey, error) {
	if candidate == nil || candidate.UserID <= 0 || strings.TrimSpace(candidate.Key) == "" || strings.TrimSpace(purpose) == "" {
		return nil, fmt.Errorf("invalid system api key candidate")
	}

	var result *service.APIKey
	err := r.withTx(ctx, func(txCtx context.Context, client *dbent.Client) error {
		executor, ok := client.Driver().(sqlExecutor)
		if !ok {
			return service.ErrSystemAPIKeyStoreUnavailable
		}

		boundID, bound, err := querySystemAPIKeyBindingID(txCtx, executor, candidate.UserID, purpose)
		if err != nil {
			return wrapSystemAPIKeyStoreError(err)
		}
		if bound {
			existing, loadErr := loadActiveSystemAPIKey(txCtx, client, candidate.UserID, boundID, purpose)
			if loadErr == nil {
				result = existing
				return nil
			}
			if !errors.Is(loadErr, service.ErrAPIKeyNotFound) {
				return loadErr
			}
		}

		if err := r.createAPIKeyInTx(txCtx, client, candidate); err != nil {
			return err
		}

		wonBinding := false
		if bound {
			updateResult, updateErr := executor.ExecContext(txCtx, `
				UPDATE system_api_key_bindings
				SET api_key_id = $1, updated_at = CURRENT_TIMESTAMP
				WHERE user_id = $2 AND purpose = $3 AND api_key_id = $4`,
				candidate.ID, candidate.UserID, purpose, boundID)
			if updateErr != nil {
				return wrapSystemAPIKeyStoreError(updateErr)
			}
			affected, affectedErr := updateResult.RowsAffected()
			if affectedErr != nil {
				return affectedErr
			}
			wonBinding = affected == 1
		} else {
			insertResult, insertErr := executor.ExecContext(txCtx, `
				INSERT INTO system_api_key_bindings (user_id, api_key_id, purpose)
				VALUES ($1, $2, $3)
				ON CONFLICT (user_id, purpose) DO NOTHING`,
				candidate.UserID, candidate.ID, purpose)
			if insertErr != nil {
				return wrapSystemAPIKeyStoreError(insertErr)
			}
			affected, affectedErr := insertResult.RowsAffected()
			if affectedErr != nil {
				return affectedErr
			}
			wonBinding = affected == 1
		}

		if wonBinding {
			candidate.ManagedType = purpose
			result = candidate
			return nil
		}

		// Another instance reconciled the same user/purpose first. Remove this
		// transaction's unused candidate and return the winning key.
		if err := softDeleteSystemAPIKeyCandidate(txCtx, client, candidate.ID); err != nil {
			return err
		}
		winnerID, found, err := querySystemAPIKeyBindingID(txCtx, executor, candidate.UserID, purpose)
		if err != nil {
			return wrapSystemAPIKeyStoreError(err)
		}
		if !found {
			return fmt.Errorf("system api key binding disappeared during reconciliation")
		}
		winner, err := loadActiveSystemAPIKey(txCtx, client, candidate.UserID, winnerID, purpose)
		if err != nil {
			return err
		}
		result = winner
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
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
	rows, err := r.sql.QueryContext(ctx, `
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

func (r *apiKeyRepository) GetSystemAPIKeyPurpose(ctx context.Context, userID, keyID int64) (string, bool, error) {
	if r.sql == nil {
		return "", false, service.ErrSystemAPIKeyStoreUnavailable
	}
	rows, err := r.sql.QueryContext(ctx, `
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

func querySystemAPIKeyBindingID(ctx context.Context, executor sqlExecutor, userID int64, purpose string) (int64, bool, error) {
	rows, err := executor.QueryContext(ctx, `
		SELECT api_key_id
		FROM system_api_key_bindings
		WHERE user_id = $1 AND purpose = $2
		LIMIT 1`, userID, purpose)
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

func loadActiveSystemAPIKey(ctx context.Context, client *dbent.Client, userID, keyID int64, purpose string) (*service.APIKey, error) {
	entity, err := client.APIKey.Query().
		Where(apikey.IDEQ(keyID), apikey.UserIDEQ(userID), apikey.DeletedAtIsNil()).
		WithUser().
		WithGroup().
		WithGroupRoutes(apiKeyGroupRouteQueryOptions).
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
	deletedAt := time.Now()
	tombstoneKey := fmt.Sprintf("__deleted_system__%d__%d", keyID, deletedAt.UnixNano())
	affected, err := client.APIKey.Update().
		Where(apikey.IDEQ(keyID), apikey.DeletedAtIsNil()).
		SetKey(tombstoneKey).
		SetDeletedAt(deletedAt).
		Save(ctx)
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
		(strings.Contains(message, "does not exist") ||
			strings.Contains(message, "no such table") ||
			strings.Contains(message, "undefined table")) {
		return fmt.Errorf("%w: %v", service.ErrSystemAPIKeyStoreUnavailable, err)
	}
	return err
}

var _ service.SystemAPIKeyRepository = (*apiKeyRepository)(nil)
