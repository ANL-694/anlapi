\set ON_ERROR_STOP on

WITH credential_keys AS (
  SELECT DISTINCT a.id,
    replace(replace(lower(trim(k.key)), '-', '_'), '.', '_') AS normalized
  FROM accounts a
  CROSS JOIN LATERAL jsonb_object_keys(COALESCE(a.credentials, '{}'::jsonb)) AS k(key)
  WHERE a.deleted_at IS NULL AND a.type IN ('oauth', 'setup-token')
)
SELECT 'oauth_sensitive_rows=' || count(DISTINCT id)
FROM credential_keys
WHERE normalized IN (
    'access_token', 'refresh_token', 'id_token', 'oauth_token',
    'oauth_access_token', 'oauth_refresh_token', 'session_token',
    'session_key', 'claude_session_key', 'cookie', 'cookies',
    'browser_cookie', 'set_cookie', 'client_secret', 'authorization',
    'authorization_header'
  )
  OR normalized LIKE '%\_secret' ESCAPE '\'
  OR (
    normalized LIKE '%token%'
    AND normalized NOT IN ('token_type', 'oauth_type', 'token_version', '_token_version', 'oauth_token_version')
  );

SELECT 'oauth_accounts=' || count(*)
FROM accounts
WHERE deleted_at IS NULL AND type IN ('oauth', 'setup-token');

SELECT 'oauth_marker_rows=' || count(*)
FROM accounts
WHERE deleted_at IS NULL
  AND type IN ('oauth', 'setup-token')
  AND credentials ? '_oauth_vault';
