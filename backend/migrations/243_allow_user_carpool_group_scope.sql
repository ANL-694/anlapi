-- Reconcile the ownership constraints introduced by migration 222 with the
-- user-carpool scope introduced earlier by migrations 186-187.
-- This is a forward-only repair; existing migration files remain immutable.

ALTER TABLE groups
    DROP CONSTRAINT IF EXISTS groups_private_owner_scope_check,
    DROP CONSTRAINT IF EXISTS groups_scope_b07b_check;

ALTER TABLE groups
    ADD CONSTRAINT groups_private_owner_scope_check
    CHECK (
        (scope = 'public' AND owner_user_id IS NULL)
        OR
        (scope IN ('user_private', 'user_carpool') AND owner_user_id IS NOT NULL)
    ),
    ADD CONSTRAINT groups_scope_b07b_check
    CHECK (scope IN ('public', 'user_private', 'user_carpool'));
