-- Keep an owned account reachable from its owner's private group after public sharing.
-- This is idempotent and leaves existing public bindings unchanged.
INSERT INTO account_groups (account_id, group_id, priority, created_at)
SELECT
    account.id,
    private_group.id,
    1,
    NOW()
FROM accounts AS account
JOIN groups AS private_group
    ON private_group.owner_user_id = account.owner_user_id
    AND private_group.platform = account.platform
    AND private_group.scope = 'user_private'
    AND private_group.deleted_at IS NULL
WHERE account.owner_user_id IS NOT NULL
    AND account.share_mode = 'public'
    AND account.deleted_at IS NULL
    AND NOT EXISTS (
        SELECT 1
        FROM account_groups AS binding
        WHERE binding.account_id = account.id
            AND binding.group_id = private_group.id
    )
ON CONFLICT (account_id, group_id) DO NOTHING;
