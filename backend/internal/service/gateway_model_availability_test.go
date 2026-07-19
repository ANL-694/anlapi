//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestDiagnoseModelAvailabilityForPlatform_IgnoresTransientAccountState(t *testing.T) {
	groupID := int64(42)
	cooldownUntil := time.Now().Add(time.Hour)
	account := Account{
		ID:                     1,
		Platform:               PlatformAnthropic,
		Status:                 StatusActive,
		Schedulable:            true,
		RateLimitResetAt:       &cooldownUntil,
		OverloadUntil:          &cooldownUntil,
		TempUnschedulableUntil: &cooldownUntil,
		AccountGroups:          []AccountGroup{{GroupID: groupID}},
		Credentials: map[string]any{
			"model_mapping": map[string]any{"claude-opus-4-8": "claude-opus-4-8"},
		},
	}
	repo := &mockAccountRepoForPlatform{
		accounts:     []Account{account},
		accountsByID: map[int64]*Account{account.ID: &account},
	}
	svc := &GatewayService{accountRepo: repo, cfg: testConfig(), schedulerSnapshot: &SchedulerSnapshotService{}}

	diagnosis := svc.DiagnoseModelAvailabilityForPlatform(context.Background(), &groupID, "claude-opus-4-8", PlatformAnthropic)

	require.True(t, diagnosis.HasAccountsInPool)
	require.True(t, diagnosis.HasModelSupport)
}

func TestOpenAIDiagnoseModelAvailabilityForPlatform_IgnoresTransientAccountState(t *testing.T) {
	groupID := int64(43)
	cooldownUntil := time.Now().Add(time.Hour)
	account := Account{
		ID:                     2,
		Platform:               PlatformOpenAI,
		Status:                 StatusActive,
		Schedulable:            true,
		RateLimitResetAt:       &cooldownUntil,
		OverloadUntil:          &cooldownUntil,
		TempUnschedulableUntil: &cooldownUntil,
		AccountGroups:          []AccountGroup{{GroupID: groupID}},
		Credentials: map[string]any{
			"model_mapping": map[string]any{"gpt-5.6": "gpt-5.6"},
		},
	}
	repo := &mockAccountRepoForPlatform{
		accounts:     []Account{account},
		accountsByID: map[int64]*Account{account.ID: &account},
	}
	svc := &OpenAIGatewayService{accountRepo: repo, cfg: testConfig(), schedulerSnapshot: &SchedulerSnapshotService{}}

	diagnosis := svc.DiagnoseModelAvailabilityForPlatform(context.Background(), &groupID, "gpt-5.6", PlatformOpenAI)

	require.True(t, diagnosis.HasAccountsInPool)
	require.True(t, diagnosis.HasModelSupport)
}
