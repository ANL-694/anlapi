-- Prerequisites for the user-private and user-carpool group migrations.
-- Keep this as a new migration: 186 references these columns before the
-- later account-isolation migration (222) runs.

ALTER TABLE groups
    ADD COLUMN IF NOT EXISTS owner_user_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS scope VARCHAR(20) NOT NULL DEFAULT 'public';

UPDATE groups
SET scope = 'public'
WHERE scope IS NULL OR BTRIM(scope) = '';
