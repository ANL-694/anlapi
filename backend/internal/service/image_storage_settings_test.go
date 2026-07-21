package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"

	"anlapi/internal/config"
	"github.com/stretchr/testify/require"
)

type recordingImageStorage struct {
	testConnectionErr error
}

func (recordingImageStorage) Save(_ context.Context, key, _ string, _ []byte) (string, error) {
	return "https://cdn.example.com/" + key, nil
}

func (s recordingImageStorage) TestConnection(context.Context) error {
	return s.testConnectionErr
}

type imageStorageSettingsTestRepo struct {
	mu          sync.Mutex
	values      map[string]string
	getValueErr error
}

func (r *imageStorageSettingsTestRepo) Get(context.Context, string) (*Setting, error) {
	return nil, nil
}
func (r *imageStorageSettingsTestRepo) GetValue(_ context.Context, key string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.getValueErr != nil {
		return "", r.getValueErr
	}
	return r.values[key], nil
}
func (r *imageStorageSettingsTestRepo) Set(_ context.Context, key, value string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.values[key] = value
	return nil
}
func (r *imageStorageSettingsTestRepo) GetMultiple(context.Context, []string) (map[string]string, error) {
	return map[string]string{}, nil
}
func (r *imageStorageSettingsTestRepo) SetMultiple(context.Context, map[string]string) error {
	return nil
}
func (r *imageStorageSettingsTestRepo) GetAll(context.Context) (map[string]string, error) {
	return map[string]string{}, nil
}
func (r *imageStorageSettingsTestRepo) Delete(context.Context, string) error { return nil }

type imageStorageSettingsTestEncryptor struct{}

func (imageStorageSettingsTestEncryptor) Encrypt(plaintext string) (string, error) {
	return "ENC:" + plaintext, nil
}
func (imageStorageSettingsTestEncryptor) Decrypt(ciphertext string) (string, error) {
	if !strings.HasPrefix(ciphertext, "ENC:") {
		return "", errors.New("not encrypted")
	}
	return strings.TrimPrefix(ciphertext, "ENC:"), nil
}

func newImageStorageSettingsFixture(t *testing.T, fixedKey bool) (*ImageStorageSettingService, *imageStorageSettingsTestRepo, *[]config.ImageStorageConfig) {
	t.Helper()
	repo := &imageStorageSettingsTestRepo{values: map[string]string{}}
	backup := NewBackupService(repo, &config.Config{
		Totp: config.TotpConfig{EncryptionKeyConfigured: fixedKey},
	}, imageStorageSettingsTestEncryptor{}, nil, nil)
	var built []config.ImageStorageConfig
	factory := func(_ context.Context, cfg *config.ImageStorageConfig) (ImageStorage, error) {
		built = append(built, *cfg)
		return recordingImageStorage{}, nil
	}
	return NewImageStorageSettingService(repo, imageStorageSettingsTestEncryptor{}, backup, factory, config.ImageStorageConfig{}), repo, &built
}

func TestImageStorageSettingsToggleTakesEffectWithoutRestart(t *testing.T) {
	svc, repo, built := newImageStorageSettingsFixture(t, true)
	backupCfg := BackupS3Config{
		Endpoint: "https://acct.r2.cloudflarestorage.com", Region: "auto", Bucket: "backup-bucket",
		AccessKeyID: "ak", SecretAccessKey: "ENC:sk", Prefix: "backups/",
	}
	raw, err := json.Marshal(backupCfg)
	require.NoError(t, err)
	require.NoError(t, repo.Set(context.Background(), settingKeyBackupS3Config, string(raw)))

	_, enabled := svc.resolve()
	require.False(t, enabled)

	_, err = svc.Update(context.Background(), ImageStorageSettings{Enabled: true, ReuseBackupS3: true})
	require.NoError(t, err)
	_, enabled = svc.resolve()
	require.True(t, enabled)

	_, err = svc.Update(context.Background(), ImageStorageSettings{Enabled: false, ReuseBackupS3: true})
	require.NoError(t, err)
	_, enabled = svc.resolve()
	require.False(t, enabled)
	require.Len(t, *built, 1)
}

func TestImageStorageSettingsEncryptAndMaskOwnSecret(t *testing.T) {
	svc, repo, built := newImageStorageSettingsFixture(t, true)
	saved, err := svc.Update(context.Background(), ImageStorageSettings{
		Enabled: true, Bucket: "my-images", Endpoint: "https://acct.r2.cloudflarestorage.com",
		AccessKeyID: "ak", SecretAccessKey: "super-secret",
	})
	require.NoError(t, err)
	require.Empty(t, saved.SecretAccessKey)

	raw, err := repo.GetValue(context.Background(), settingKeyImageStorageConfig)
	require.NoError(t, err)
	require.NotContains(t, raw, `"secret_access_key":"super-secret"`)
	require.Contains(t, raw, "ENC:super-secret")

	_, enabled := svc.resolve()
	require.True(t, enabled)
	require.Equal(t, "super-secret", (*built)[0].SecretAccessKey)
}

func TestImageStorageSettingsFirstSaveEncryptsAndMasksFallbackSecret(t *testing.T) {
	for _, seedEmptySettings := range []bool{false, true} {
		name := "no database settings"
		if seedEmptySettings {
			name = "database settings without secret"
		}
		t.Run(name, func(t *testing.T) {
			svc, repo, built := newImageStorageSettingsFixture(t, true)
			svc.fallback = config.ImageStorageConfig{SecretAccessKey: "env-fallback-secret"}
			if seedEmptySettings {
				raw, err := json.Marshal(ImageStorageSettings{Enabled: false, Bucket: "old-bucket"})
				require.NoError(t, err)
				require.NoError(t, repo.Set(context.Background(), settingKeyImageStorageConfig, string(raw)))
			}
			require.True(t, svc.SecretConfigured(context.Background()))

			saved, err := svc.Update(context.Background(), ImageStorageSettings{
				Enabled: true, Bucket: "new-bucket", Endpoint: "https://acct.r2.cloudflarestorage.com",
				AccessKeyID: "new-ak",
			})
			require.NoError(t, err)
			require.Empty(t, saved.SecretAccessKey)
			savedJSON, err := json.Marshal(saved)
			require.NoError(t, err)
			require.NotContains(t, string(savedJSON), "env-fallback-secret")

			got, err := svc.Get(context.Background())
			require.NoError(t, err)
			require.Empty(t, got.SecretAccessKey)
			gotJSON, err := json.Marshal(got)
			require.NoError(t, err)
			require.NotContains(t, string(gotJSON), "env-fallback-secret")

			raw, err := repo.GetValue(context.Background(), settingKeyImageStorageConfig)
			require.NoError(t, err)
			var persisted ImageStorageSettings
			require.NoError(t, json.Unmarshal([]byte(raw), &persisted))
			require.Equal(t, "ENC:env-fallback-secret", persisted.SecretAccessKey)

			_, enabled := svc.resolve()
			require.True(t, enabled)
			require.Len(t, *built, 1)
			require.Equal(t, "env-fallback-secret", (*built)[0].SecretAccessKey)
		})
	}
}

func TestImageStorageSettingsTestConnectionReusesFallbackSecret(t *testing.T) {
	for _, seedEmptySettings := range []bool{false, true} {
		name := "no database settings"
		if seedEmptySettings {
			name = "database settings without secret"
		}
		t.Run(name, func(t *testing.T) {
			svc, repo, built := newImageStorageSettingsFixture(t, true)
			svc.fallback = config.ImageStorageConfig{SecretAccessKey: "env-fallback-secret"}
			if seedEmptySettings {
				raw, err := json.Marshal(ImageStorageSettings{Enabled: false, Bucket: "old-bucket"})
				require.NoError(t, err)
				require.NoError(t, repo.Set(context.Background(), settingKeyImageStorageConfig, string(raw)))
			}
			before := repo.values[settingKeyImageStorageConfig]

			err := svc.TestConnection(context.Background(), ImageStorageSettings{
				Enabled: true, Bucket: "new-bucket", Endpoint: "https://acct.r2.cloudflarestorage.com",
				AccessKeyID: "new-ak",
			})
			require.NoError(t, err)
			require.Len(t, *built, 1)
			require.Equal(t, "env-fallback-secret", (*built)[0].SecretAccessKey)
			require.Equal(t, before, repo.values[settingKeyImageStorageConfig], "testing a connection must not persist the fallback secret")
		})
	}
}

func TestImageStorageSettingsTestConnectionReturnsProbeError(t *testing.T) {
	svc, _, _ := newImageStorageSettingsFixture(t, true)
	want := errors.New("bucket is unreachable")
	svc.factory = func(context.Context, *config.ImageStorageConfig) (ImageStorage, error) {
		return recordingImageStorage{testConnectionErr: want}, nil
	}

	err := svc.TestConnection(context.Background(), ImageStorageSettings{
		Enabled: true, Bucket: "my-images", Endpoint: "https://storage.example.com",
		AccessKeyID: "ak", SecretAccessKey: "sk",
	})

	require.ErrorIs(t, err, want)
}

func TestImageStorageSettingsLoadReturnsRepositoryError(t *testing.T) {
	svc, repo, _ := newImageStorageSettingsFixture(t, true)
	want := errors.New("database unavailable")
	repo.getValueErr = want

	_, err := svc.Get(context.Background())

	require.ErrorIs(t, err, want)
}

func TestImageStorageSettingsRejectFallbackSecretWithEphemeralKey(t *testing.T) {
	svc, repo, _ := newImageStorageSettingsFixture(t, false)
	svc.fallback = config.ImageStorageConfig{SecretAccessKey: "env-fallback-secret"}

	_, err := svc.Update(context.Background(), ImageStorageSettings{
		Enabled: true, Bucket: "my-images", Endpoint: "https://acct.r2.cloudflarestorage.com",
		AccessKeyID: "ak",
	})
	require.ErrorIs(t, err, ErrSecretEncryptionKeyNotConfigured)
	require.Empty(t, repo.values)
}

func TestImageStorageSettingsRejectSecretWithEphemeralKey(t *testing.T) {
	svc, repo, _ := newImageStorageSettingsFixture(t, false)
	_, err := svc.Update(context.Background(), ImageStorageSettings{
		Enabled: true, Bucket: "my-images", Endpoint: "https://acct.r2.cloudflarestorage.com",
		AccessKeyID: "ak", SecretAccessKey: "super-secret",
	})
	require.ErrorIs(t, err, ErrSecretEncryptionKeyNotConfigured)
	raw, _ := repo.GetValue(context.Background(), settingKeyImageStorageConfig)
	require.Empty(t, raw)
}

func TestBackupServiceRejectsEphemeralKey(t *testing.T) {
	repo := &imageStorageSettingsTestRepo{values: map[string]string{}}
	svc := NewBackupService(repo, &config.Config{
		Totp: config.TotpConfig{EncryptionKeyConfigured: false},
	}, imageStorageSettingsTestEncryptor{}, nil, nil)

	_, err := svc.UpdateS3Config(context.Background(), BackupS3Config{
		Bucket: "my-bucket", AccessKeyID: "ak", SecretAccessKey: "secret",
	})
	require.ErrorIs(t, err, ErrSecretEncryptionKeyNotConfigured)
	require.False(t, svc.EncryptionKeyConfigured())
	require.Empty(t, repo.values)
}
