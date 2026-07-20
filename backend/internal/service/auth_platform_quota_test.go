package service

import (
	"context"
	"errors"
	"testing"
	"time"

	dbent "anl-api/ent"
	"anl-api/internal/config"

	"github.com/stretchr/testify/require"
)

type authPlatformQuotaRepo struct {
	records []UserPlatformQuotaRecord
	lastCtx context.Context
	err     error
}

func (r *authPlatformQuotaRepo) GetByUserPlatform(context.Context, int64, string) (*UserPlatformQuotaRecord, error) {
	return nil, nil
}
func (r *authPlatformQuotaRepo) BulkInsertInitial(ctx context.Context, records []UserPlatformQuotaRecord) error {
	r.lastCtx = ctx
	r.records = append(r.records, records...)
	return r.err
}
func (r *authPlatformQuotaRepo) IncrementUsageWithReset(context.Context, int64, string, float64, time.Time) error {
	return nil
}
func (r *authPlatformQuotaRepo) ListByUser(context.Context, int64) ([]UserPlatformQuotaRecord, error) {
	return nil, nil
}
func (r *authPlatformQuotaRepo) UpsertForUser(context.Context, int64, []UserPlatformQuotaRecord) error {
	return nil
}
func (r *authPlatformQuotaRepo) ResetExpiredWindow(context.Context, int64, string, string, time.Time) error {
	return nil
}
func (r *authPlatformQuotaRepo) BatchSnapshotUsage(context.Context, []UserPlatformQuotaSnapshot, time.Time) error {
	return nil
}

func TestSnapshotPlatformQuotaDefaults(t *testing.T) {
	daily := 5.0
	repo := &authPlatformQuotaRepo{}
	service := &AuthService{userPlatformQuotaRepo: repo}
	txCtx := dbent.NewTxContext(context.Background(), &dbent.Tx{})

	err := service.snapshotPlatformQuotaDefaults(txCtx, 99, &signupGrantPlan{
		PlatformQuotas: map[string]*DefaultPlatformQuotaSetting{
			PlatformAnthropic: {DailyLimitUSD: &daily},
			PlatformOpenAI:    {},
		},
	})
	require.NoError(t, err)
	require.Len(t, repo.records, 2)
	require.Nil(t, dbent.TxFromContext(repo.lastCtx), "quota snapshot must not inherit the registration transaction")
	require.Equal(t, int64(99), repo.records[0].UserID)
}

func TestSnapshotPlatformQuotaDefaultsFailsOpen(t *testing.T) {
	repo := &authPlatformQuotaRepo{err: errors.New("db unavailable")}
	service := &AuthService{userPlatformQuotaRepo: repo}
	require.NoError(t, service.snapshotPlatformQuotaDefaults(context.Background(), 1, &signupGrantPlan{
		PlatformQuotas: map[string]*DefaultPlatformQuotaSetting{PlatformOpenAI: {}},
	}))
}

func TestResolveSignupGrantPlanMergesPlatformQuotaDefaults(t *testing.T) {
	repo := &platformQuotaSettingRepo{data: map[string]string{
		SettingKeyDefaultPlatformQuotas:                  `{"openai":{"daily":10,"monthly":100}}`,
		SettingKeyAuthSourceDefaultEmailGrantOnSignup:    "true",
		SettingKeyAuthSourcePlatformQuotas("email"):      `{"openai":{"daily":3}}`,
		SettingKeyAuthSourceDefaultEmailBalance:          "0",
		SettingKeyAuthSourceDefaultEmailConcurrency:      "5",
		SettingKeyAuthSourceDefaultEmailSubscriptions:    "[]",
		SettingKeyAuthSourceDefaultEmailGrantOnFirstBind: "false",
	}}
	settingService := NewSettingService(repo, &config.Config{})
	authService := &AuthService{settingService: settingService, cfg: &config.Config{}}

	plan := authService.resolveSignupGrantPlan(context.Background(), "email")
	require.NotNil(t, plan.PlatformQuotas[PlatformOpenAI])
	require.Equal(t, 3.0, *plan.PlatformQuotas[PlatformOpenAI].DailyLimitUSD)
	require.Equal(t, 100.0, *plan.PlatformQuotas[PlatformOpenAI].MonthlyLimitUSD)
	require.Contains(t, plan.PlatformQuotas, PlatformKiro)
}
