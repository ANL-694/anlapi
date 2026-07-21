package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"

	"anlapi/internal/config"
	"anlapi/internal/pkg/logger"
	"go.uber.org/zap"
)

const settingKeyImageStorageConfig = "image_storage_config"

var ErrImageStorageIncomplete = errors.New("image storage is enabled but bucket/access_key_id/secret_access_key are incomplete")

// ImageStorageFactory 由 repository 层提供，避免 service 反向依赖具体 S3 实现。
type ImageStorageFactory func(ctx context.Context, cfg *config.ImageStorageConfig) (ImageStorage, error)

// ImageStorageSettings 是后台可编辑的异步生图对象存储配置。
// ReuseBackupS3 为真时只保存图片自己的存储桶和前缀，凭证复用备份 S3。
type ImageStorageSettings struct {
	Enabled       bool `json:"enabled"`
	ReuseBackupS3 bool `json:"reuse_backup_s3"`

	Bucket           string `json:"bucket"`
	Prefix           string `json:"prefix"`
	PublicBaseURL    string `json:"public_base_url"`
	PresignExpiry    int    `json:"presign_expiry_hours"`
	MaxDownloadBytes int64  `json:"max_download_bytes"`

	Endpoint        string `json:"endpoint"`
	Region          string `json:"region"`
	AccessKeyID     string `json:"access_key_id"`
	SecretAccessKey string `json:"secret_access_key,omitempty"` //nolint:revive // field name follows AWS convention
	ForcePathStyle  bool   `json:"force_path_style"`
}

// ImageStorageSettingService 将后台设置解析为运行时 uploader；保存后清缓存，
// 下一次异步生图请求即使用新配置，无需重启。
type ImageStorageSettingService struct {
	settingRepo SettingRepository
	encryptor   SecretEncryptor
	backup      *BackupService
	factory     ImageStorageFactory
	fallback    config.ImageStorageConfig

	mu       sync.Mutex
	resolved bool
	uploader *ImageResultUploader
	enabled  bool
}

func NewImageStorageSettingService(
	settingRepo SettingRepository,
	encryptor SecretEncryptor,
	backup *BackupService,
	factory ImageStorageFactory,
	fallback config.ImageStorageConfig,
) *ImageStorageSettingService {
	return &ImageStorageSettingService{
		settingRepo: settingRepo,
		encryptor:   encryptor,
		backup:      backup,
		factory:     factory,
		fallback:    fallback,
	}
}

func (s *ImageStorageSettingService) Resolver() ImageStorageResolver {
	return func() (*ImageResultUploader, bool) { return s.resolve() }
}

func (s *ImageStorageSettingService) resolve() (*ImageResultUploader, bool) {
	if s == nil {
		return nil, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.resolved {
		return s.uploader, s.enabled
	}

	s.resolved = true
	s.uploader, s.enabled = nil, false
	cfg, err := s.effectiveConfig(context.Background())
	if err != nil {
		logger.L().Warn("image_storage.settings_load_failed; async image tasks stay disabled", zap.Error(err))
		return nil, false
	}
	if !cfg.Enabled {
		return nil, false
	}
	if !cfg.IsConfigured() {
		logger.L().Warn("image_storage is enabled but not fully configured; async image tasks are disabled", zap.Strings("missing_keys", cfg.MissingCredentialKeys()))
		return nil, false
	}
	storage, err := s.factory(context.Background(), cfg)
	if err != nil {
		logger.L().Error("image_storage.client_build_failed; async image tasks stay disabled", zap.Error(err))
		return nil, false
	}
	s.uploader = NewImageResultUploader(storage, cfg.Prefix, cfg.MaxDownloadByte, nil)
	s.enabled = true
	return s.uploader, true
}

func (s *ImageStorageSettingService) Invalidate() {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.resolved = false
	s.uploader = nil
	s.enabled = false
	s.mu.Unlock()
}

func (s *ImageStorageSettingService) Get(ctx context.Context) (*ImageStorageSettings, error) {
	settings, err := s.load(ctx)
	if err != nil {
		return nil, err
	}
	if settings == nil {
		settings = settingsFromConfig(s.fallback)
	}
	settings.SecretAccessKey = ""
	return settings, nil
}

func (s *ImageStorageSettingService) SecretConfigured(ctx context.Context) bool {
	settings, err := s.load(ctx)
	if err != nil || settings == nil {
		return s.fallback.SecretAccessKey != ""
	}
	if settings.ReuseBackupS3 {
		cfg, err := s.backupCredentials(ctx)
		return err == nil && cfg != nil && cfg.SecretAccessKey != ""
	}
	return settings.SecretAccessKey != "" || s.fallback.SecretAccessKey != ""
}

// Update 保存设置并立即使缓存失效。空 SecretAccessKey 表示保留已存值。
func (s *ImageStorageSettingService) Update(ctx context.Context, in ImageStorageSettings) (*ImageStorageSettings, error) {
	normalizeImageStorageSettings(&in)
	if in.ReuseBackupS3 {
		in.Endpoint, in.Region, in.AccessKeyID, in.SecretAccessKey = "", "", "", ""
		in.ForcePathStyle = false
	} else if in.SecretAccessKey == "" {
		old, err := s.load(ctx)
		if err != nil {
			return nil, err
		}
		if old != nil && old.SecretAccessKey != "" {
			in.SecretAccessKey = old.SecretAccessKey
		} else if s.fallback.SecretAccessKey != "" {
			in.SecretAccessKey, err = s.encryptSecret(s.fallback.SecretAccessKey)
			if err != nil {
				return nil, err
			}
		}
	} else {
		encrypted, err := s.encryptSecret(in.SecretAccessKey)
		if err != nil {
			return nil, err
		}
		in.SecretAccessKey = encrypted
	}

	data, err := json.Marshal(in)
	if err != nil {
		return nil, fmt.Errorf("marshal image storage settings: %w", err)
	}
	if err := s.settingRepo.Set(ctx, settingKeyImageStorageConfig, string(data)); err != nil {
		return nil, fmt.Errorf("save image storage settings: %w", err)
	}
	s.Invalidate()
	in.SecretAccessKey = ""
	return &in, nil
}

func (s *ImageStorageSettingService) TestConnection(ctx context.Context, in ImageStorageSettings) error {
	normalizeImageStorageSettings(&in)
	if !in.ReuseBackupS3 && in.SecretAccessKey == "" {
		old, err := s.load(ctx)
		if err != nil {
			return err
		}
		if old != nil && old.SecretAccessKey != "" {
			in.SecretAccessKey = old.SecretAccessKey
		}
	}
	cfg, err := s.toImageStorageConfig(ctx, &in)
	if err != nil {
		return err
	}
	if !cfg.IsConfigured() {
		return ErrImageStorageIncomplete
	}
	storage, err := s.factory(ctx, cfg)
	if err != nil {
		return err
	}
	tester, ok := storage.(ImageStorageConnectionTester)
	if !ok {
		return errors.New("image storage backend does not support connection testing")
	}
	return tester.TestConnection(ctx)
}

func (s *ImageStorageSettingService) effectiveConfig(ctx context.Context) (*config.ImageStorageConfig, error) {
	settings, err := s.load(ctx)
	if err != nil {
		return nil, err
	}
	if settings == nil {
		fallback := s.fallback
		return &fallback, nil
	}
	return s.toImageStorageConfig(ctx, settings)
}

func (s *ImageStorageSettingService) toImageStorageConfig(ctx context.Context, in *ImageStorageSettings) (*config.ImageStorageConfig, error) {
	cfg := &config.ImageStorageConfig{
		Enabled: in.Enabled, Bucket: in.Bucket, Prefix: in.Prefix, PublicBaseURL: in.PublicBaseURL,
		PresignExpiry: in.PresignExpiry, MaxDownloadByte: in.MaxDownloadBytes, Endpoint: in.Endpoint,
		Region: in.Region, AccessKeyID: in.AccessKeyID, SecretAccessKey: in.SecretAccessKey, ForcePathStyle: in.ForcePathStyle,
	}
	if in.ReuseBackupS3 {
		backupCfg, err := s.backupCredentials(ctx)
		if err != nil {
			return nil, err
		}
		if backupCfg == nil {
			return nil, errors.New("image storage is set to reuse the backup S3 configuration, but no backup S3 configuration exists")
		}
		cfg.Endpoint = backupCfg.Endpoint
		cfg.Region = backupCfg.Region
		cfg.AccessKeyID = backupCfg.AccessKeyID
		cfg.SecretAccessKey = backupCfg.SecretAccessKey
		cfg.ForcePathStyle = backupCfg.ForcePathStyle
		if cfg.Bucket == "" {
			cfg.Bucket = backupCfg.Bucket
		}
	} else if cfg.SecretAccessKey != "" {
		decrypted, err := s.encryptor.Decrypt(cfg.SecretAccessKey)
		if err != nil {
			logger.L().Warn("image_storage secret decrypt failed; treating the stored value as plaintext", zap.Error(err))
		} else {
			cfg.SecretAccessKey = decrypted
		}
	} else {
		cfg.SecretAccessKey = s.fallback.SecretAccessKey
	}
	return cfg, nil
}

func (s *ImageStorageSettingService) encryptSecret(secret string) (string, error) {
	if s.backup == nil || !s.backup.EncryptionKeyConfigured() {
		return "", ErrSecretEncryptionKeyNotConfigured
	}
	encrypted, err := s.encryptor.Encrypt(secret)
	if err != nil {
		return "", fmt.Errorf("encrypt image storage secret: %w", err)
	}
	return encrypted, nil
}

func (s *ImageStorageSettingService) backupCredentials(ctx context.Context) (*BackupS3Config, error) {
	if s.backup == nil {
		return nil, errors.New("backup service is unavailable")
	}
	return s.backup.loadS3Config(ctx)
}

func (s *ImageStorageSettingService) load(ctx context.Context) (*ImageStorageSettings, error) {
	if s.settingRepo == nil {
		return nil, nil
	}
	raw, err := s.settingRepo.GetValue(ctx, settingKeyImageStorageConfig)
	if err != nil {
		if errors.Is(err, ErrSettingNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("load image storage settings: %w", err)
	}
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	var settings ImageStorageSettings
	if err := json.Unmarshal([]byte(raw), &settings); err != nil {
		return nil, fmt.Errorf("parse image storage settings: %w", err)
	}
	return &settings, nil
}

func settingsFromConfig(cfg config.ImageStorageConfig) *ImageStorageSettings {
	return &ImageStorageSettings{
		Enabled: cfg.Enabled, Bucket: cfg.Bucket, Prefix: cfg.Prefix, PublicBaseURL: cfg.PublicBaseURL,
		PresignExpiry: cfg.PresignExpiry, MaxDownloadBytes: cfg.MaxDownloadByte, Endpoint: cfg.Endpoint,
		Region: cfg.Region, AccessKeyID: cfg.AccessKeyID, SecretAccessKey: cfg.SecretAccessKey, ForcePathStyle: cfg.ForcePathStyle,
	}
}

func normalizeImageStorageSettings(in *ImageStorageSettings) {
	in.Bucket = strings.TrimSpace(in.Bucket)
	in.Endpoint = strings.TrimSpace(in.Endpoint)
	in.Region = strings.TrimSpace(in.Region)
	in.AccessKeyID = strings.TrimSpace(in.AccessKeyID)
	in.SecretAccessKey = strings.TrimSpace(in.SecretAccessKey)
	in.PublicBaseURL = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(in.PublicBaseURL), "/"))
	in.Prefix = strings.TrimSpace(in.Prefix)
	if in.Prefix == "" {
		in.Prefix = "images/"
	}
	if !strings.HasSuffix(in.Prefix, "/") {
		in.Prefix += "/"
	}
	if in.Region == "" {
		in.Region = "auto"
	}
	if in.PresignExpiry <= 0 {
		in.PresignExpiry = 24
	}
	if in.MaxDownloadBytes <= 0 {
		in.MaxDownloadBytes = defaultImageMaxDownloadBytes
	}
}
