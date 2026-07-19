-- 用户平台维度配额。迁移编号使用 198，避免与 IK/ANL 已存在的 142、157 冲突。
-- 可同时处理新库和已存在旧版 user_platform_quotas 表的实例。

CREATE TABLE IF NOT EXISTS user_platform_quotas (
    id                   BIGSERIAL PRIMARY KEY,
    user_id              BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    platform             VARCHAR(32) NOT NULL,
    daily_limit_usd      DECIMAL(20,10),
    weekly_limit_usd     DECIMAL(20,10),
    monthly_limit_usd    DECIMAL(20,10),
    daily_usage_usd      DECIMAL(20,10) NOT NULL DEFAULT 0,
    weekly_usage_usd     DECIMAL(20,10) NOT NULL DEFAULT 0,
    monthly_usage_usd    DECIMAL(20,10) NOT NULL DEFAULT 0,
    daily_window_start   TIMESTAMPTZ,
    weekly_window_start  TIMESTAMPTZ,
    monthly_window_start TIMESTAMPTZ,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at           TIMESTAMPTZ
);

ALTER TABLE user_platform_quotas
    DROP CONSTRAINT IF EXISTS user_platform_quotas_platform_check;

ALTER TABLE user_platform_quotas
    ADD CONSTRAINT user_platform_quotas_platform_check
    CHECK (platform IN ('anthropic', 'openai', 'gemini', 'antigravity', 'grok', 'kiro'));

CREATE UNIQUE INDEX IF NOT EXISTS userplatformquota_user_id_platform_uq
    ON user_platform_quotas (user_id, platform)
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS userplatformquota_user_id
    ON user_platform_quotas (user_id);
