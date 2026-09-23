-- B07b: persist the minimum ownership metadata needed to keep user-private
-- accounts and groups out of public scheduling paths. This migration does not
-- introduce account sharing, settlement, carpool, or subscription provisioning.

ALTER TABLE accounts
    ADD COLUMN IF NOT EXISTS owner_user_id BIGINT REFERENCES users(id) ON DELETE SET NULL;

ALTER TABLE groups
    ADD COLUMN IF NOT EXISTS owner_user_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS scope VARCHAR(20) NOT NULL DEFAULT 'public';

UPDATE groups
SET scope = 'public'
WHERE scope IS NULL OR BTRIM(scope) = '';

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'groups_private_owner_scope_check') THEN
        ALTER TABLE groups
            ADD CONSTRAINT groups_private_owner_scope_check
            CHECK (
                (scope = 'public' AND owner_user_id IS NULL)
                OR
                (scope = 'user_private' AND owner_user_id IS NOT NULL)
            );
    END IF;
END $$;

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'groups_scope_b07b_check') THEN
        ALTER TABLE groups
            ADD CONSTRAINT groups_scope_b07b_check
            CHECK (scope IN ('public', 'user_private'));
    END IF;
END $$;

CREATE INDEX IF NOT EXISTS idx_accounts_owner_user_id_b07b
    ON accounts (owner_user_id)
    WHERE deleted_at IS NULL AND owner_user_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_groups_owner_user_id_b07b
    ON groups (owner_user_id)
    WHERE deleted_at IS NULL AND owner_user_id IS NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS idx_groups_user_private_owner_platform_b07b
    ON groups (owner_user_id, platform)
    WHERE deleted_at IS NULL
      AND scope = 'user_private';
