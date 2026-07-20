#!/usr/bin/env bash
set -euo pipefail

mode="${1:-check}"
database_name="${DR_DATABASE_NAME:-ikik_api}"
replication_user="${DR_REPLICATION_USER:-anl_dr_replica}"
database_owner="${DR_DATABASE_OWNER:-ikik_api}"
app_service="${DR_APP_SERVICE:-anlapi}"
postgres_service="${DR_POSTGRES_SERVICE:-postgresql}"
script_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

run_psql() {
  sudo -u postgres psql -X -v ON_ERROR_STOP=1 -d "$database_name" "$@"
}

check_isolation() {
  local sensitive_rows
  sensitive_rows="$(run_psql -At <<'SQL'
WITH credential_keys AS (
  SELECT DISTINCT a.id,
    replace(replace(lower(trim(k.key)), '-', '_'), '.', '_') AS normalized
  FROM accounts a
  CROSS JOIN LATERAL jsonb_object_keys(COALESCE(a.credentials, '{}'::jsonb)) AS k(key)
  WHERE a.deleted_at IS NULL AND a.type IN ('oauth', 'setup-token')
)
SELECT count(DISTINCT id)
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
SQL
)"
  printf 'oauth_sensitive_rows=%s\n' "$sensitive_rows"
  if [[ "$sensitive_rows" != "0" ]]; then
    echo 'Refusing DR publication before OAuth isolation is complete.' >&2
    return 1
  fi
}

show_status() {
  run_psql -At <<'SQL'
SELECT 'server_version=' || current_setting('server_version');
SELECT 'wal_level=' || current_setting('wal_level');
SELECT 'max_replication_slots=' || current_setting('max_replication_slots');
SELECT 'max_wal_senders=' || current_setting('max_wal_senders');
SELECT 'max_slot_wal_keep_size=' || current_setting('max_slot_wal_keep_size');
SELECT 'publication_tables=' || count(*) FROM pg_publication_tables WHERE pubname = 'anl_core_publication';
SELECT 'replication_slots=' || count(*) FROM pg_replication_slots;
SQL
}

configure_postgres() {
  if [[ "${DR_CONFIRM_POSTGRES_RESTART:-}" != 'RESTART_POSTGRES' ]]; then
    echo 'Set DR_CONFIRM_POSTGRES_RESTART=RESTART_POSTGRES before configure.' >&2
    exit 2
  fi
  local backup_dir
  backup_dir="/opt/anlapi/backups/dr-postgres-$(date -u +%Y%m%dT%H%M%SZ)"
  install -d -m 0700 "$backup_dir"
  sudo -u postgres psql -X -v ON_ERROR_STOP=1 -d postgres -At <<'SQL' >"$backup_dir/before.txt"
SELECT name || '=' || setting FROM pg_settings
WHERE name IN ('wal_level', 'max_replication_slots', 'max_wal_senders', 'max_slot_wal_keep_size')
ORDER BY name;
SQL
  sudo -u postgres psql -X -v ON_ERROR_STOP=1 -d postgres <<'SQL'
ALTER SYSTEM SET wal_level = 'logical';
ALTER SYSTEM SET max_replication_slots = '10';
ALTER SYSTEM SET max_wal_senders = '10';
ALTER SYSTEM SET max_slot_wal_keep_size = '4096MB';
SQL
  systemctl restart "$postgres_service"
  systemctl restart "$app_service"
  run_psql -Atc "SELECT current_setting('wal_level')" | grep -qx logical
  systemctl is-active --quiet "$app_service"
  curl --fail --silent --show-error --max-time 15 http://127.0.0.1:8080/health >/dev/null
  printf 'postgres_config_backup=%s\n' "$backup_dir"
}

prepare_publication() {
  : "${DR_REPLICATION_PASSWORD:?DR_REPLICATION_PASSWORD is required}"
  check_isolation
  run_psql -Atc "SELECT current_setting('wal_level')" | grep -qx logical

  sudo -u postgres psql -X -v ON_ERROR_STOP=1 -d postgres \
    -v repl_role="$replication_user" \
    -v repl_password="$DR_REPLICATION_PASSWORD" <<'SQL'
SELECT format('CREATE ROLE %I LOGIN REPLICATION PASSWORD %L', :'repl_role', :'repl_password')
WHERE NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = :'repl_role') \gexec
SELECT format('ALTER ROLE %I WITH LOGIN REPLICATION PASSWORD %L', :'repl_role', :'repl_password') \gexec
SQL

  run_psql -v repl_role="$replication_user" -v database_owner="$database_owner" <<'SQL'
SELECT format('GRANT CONNECT ON DATABASE %I TO %I', current_database(), :'repl_role') \gexec
SELECT format('GRANT USAGE ON SCHEMA public TO %I', :'repl_role') \gexec
SELECT format('GRANT SELECT ON ALL TABLES IN SCHEMA public TO %I', :'repl_role') \gexec
SELECT format('ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT SELECT ON TABLES TO %I', :'repl_role') \gexec
SELECT format('ALTER DEFAULT PRIVILEGES FOR ROLE %I IN SCHEMA public GRANT SELECT ON TABLES TO %I', :'database_owner', :'repl_role') \gexec
SQL
  run_psql -f "$script_root/sql/publisher-publication.sql"
  show_status
}

case "$mode" in
  check)
    check_isolation
    show_status
    ;;
  configure)
    configure_postgres
    show_status
    ;;
  publication)
    prepare_publication
    ;;
  *)
    echo 'Usage: Prepare-AnlDrPublisher.sh [check|configure|publication]' >&2
    exit 2
    ;;
esac
