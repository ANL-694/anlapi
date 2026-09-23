-- ANL extension: automatically apply rate multipliers during configured local-time windows.
-- This combines the historical schedule and per-user-target migrations so it can be
-- introduced safely after the existing 176 replay migration set.
CREATE TABLE IF NOT EXISTS group_rate_schedules (
    id              BIGSERIAL PRIMARY KEY,
    group_id        BIGINT NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
    target_user_id  BIGINT NULL REFERENCES users(id) ON DELETE CASCADE,
    start_minute    INTEGER NOT NULL,
    end_minute      INTEGER NOT NULL,
    rate_multiplier DECIMAL(10,4) NOT NULL,
    enabled         BOOLEAN NOT NULL DEFAULT TRUE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_group_rate_schedules_start_minute
        CHECK (start_minute >= 0 AND start_minute < 1440),
    CONSTRAINT chk_group_rate_schedules_end_minute
        CHECK (end_minute > 0 AND end_minute <= 1440),
    CONSTRAINT chk_group_rate_schedules_range
        CHECK (start_minute < end_minute),
    CONSTRAINT chk_group_rate_schedules_multiplier
        CHECK (rate_multiplier > 0)
);

CREATE INDEX IF NOT EXISTS idx_group_rate_schedules_group_enabled
    ON group_rate_schedules(group_id, enabled);
CREATE INDEX IF NOT EXISTS idx_group_rate_schedules_group_target_enabled
    ON group_rate_schedules(group_id, target_user_id, enabled);

-- Entering a window saves the previous group multiplier so it can be restored
-- when no window remains active. Per-user overrides follow the same contract.
CREATE TABLE IF NOT EXISTS group_rate_schedule_states (
    group_id             BIGINT PRIMARY KEY REFERENCES groups(id) ON DELETE CASCADE,
    base_rate_multiplier DECIMAL(10,4) NOT NULL,
    applied_schedule_id  BIGINT NULL REFERENCES group_rate_schedules(id) ON DELETE SET NULL,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_group_rate_schedule_states_base_multiplier
        CHECK (base_rate_multiplier > 0)
);

CREATE TABLE IF NOT EXISTS group_rate_schedule_user_states (
    group_id             BIGINT NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
    user_id              BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    base_rate_multiplier DECIMAL(10,4) NULL,
    applied_schedule_id  BIGINT NULL REFERENCES group_rate_schedules(id) ON DELETE SET NULL,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (group_id, user_id),
    CONSTRAINT chk_group_rate_schedule_user_states_base_multiplier
        CHECK (base_rate_multiplier IS NULL OR base_rate_multiplier > 0)
);
