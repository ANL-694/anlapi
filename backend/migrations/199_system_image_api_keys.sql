CREATE TABLE IF NOT EXISTS system_api_key_bindings (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    api_key_id BIGINT NOT NULL REFERENCES api_keys(id) ON DELETE CASCADE,
    purpose VARCHAR(64) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT system_api_key_bindings_user_purpose_unique UNIQUE (user_id, purpose),
    CONSTRAINT system_api_key_bindings_key_purpose_unique UNIQUE (api_key_id, purpose)
);

CREATE INDEX IF NOT EXISTS idx_system_api_key_bindings_api_key_id
    ON system_api_key_bindings(api_key_id);
