//go:build integration

package repository

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestGetAccountSharingDashboardEnforcesCurrentAccountOwner(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	owner := mustCreateUser(t, client, &service.User{})
	consumer := mustCreateUser(t, client, &service.User{})
	account := mustCreateAccount(t, client, &service.Account{
		Name:        "sharing-owner-isolation",
		Platform:    service.PlatformOpenAI,
		Type:        service.AccountTypeAPIKey,
		OwnerUserID: &consumer.ID,
	})
	_, err := integrationDB.ExecContext(ctx, "UPDATE accounts SET share_mode = 'public', share_status = 'approved' WHERE id = $1", account.ID)
	require.NoError(t, err)
	apiKey := mustCreateApiKey(t, client, &service.APIKey{UserID: consumer.ID})

	start := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 0, 1)

	t.Cleanup(func() {
		_, _ = integrationDB.ExecContext(ctx, "DELETE FROM account_share_settlement_entries WHERE account_id = $1 OR api_key_id = $2", account.ID, apiKey.ID)
		_, _ = integrationDB.ExecContext(ctx, "DELETE FROM accounts WHERE id = $1", account.ID)
		_, _ = integrationDB.ExecContext(ctx, "DELETE FROM api_keys WHERE id = $1", apiKey.ID)
		_, _ = integrationDB.ExecContext(ctx, "DELETE FROM users WHERE id IN ($1, $2)", owner.ID, consumer.ID)
	})

	// The row is intentionally inconsistent: its snapshot says owner, but the
	// account currently belongs to consumer. Dashboard queries must fail closed.
	_, err = integrationDB.ExecContext(ctx, `
		INSERT INTO account_share_settlement_entries (
			request_id, api_key_id, consumer_user_id, owner_user_id, account_id,
			share_mode_snapshot, share_status_snapshot, consumer_charge, account_cost,
			owner_share_ratio, owner_credit, platform_fee, status, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, 'public', 'approved', 5, 4, 0.5, 2.5, 2.5, 'applied', $6, $6)
	`, "owner-mismatch", apiKey.ID, consumer.ID, owner.ID, account.ID, start.Add(12*time.Hour))
	require.NoError(t, err)

	repo := &usageLogRepository{sql: integrationDB}
	result, err := repo.GetAccountSharingDashboard(ctx, service.AccountSharingQuery{
		UserID: owner.ID, StartTime: start, EndTime: end, Granularity: "day", Page: 1, PageSize: 20,
	})
	require.NoError(t, err)
	require.Zero(t, result.Summary.ExternalRequests)
	require.Zero(t, result.Summary.ExternalConsumerCharge)
	require.Empty(t, result.Trend)
}
