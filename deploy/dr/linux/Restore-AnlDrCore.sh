#!/usr/bin/env bash
set -euo pipefail

mode="${1:-prepare}"
: "${DR_DUMP_FILE:?DR_DUMP_FILE is required}"
: "${DR_EXPECTED_SHA256:?DR_EXPECTED_SHA256 is required}"

database_name="${DR_DATABASE_NAME:-ikik_api}"
candidate_name="${DR_CANDIDATE_DATABASE:-ikik_api_dr_candidate}"
database_owner="${DR_DATABASE_OWNER:-ikik_api}"
app_binary="${DR_APP_BINARY:-/opt/ikik-api/ikik-api}"
app_service="${DR_APP_SERVICE:-ikik-api}"

actual_hash="$(sha256sum "$DR_DUMP_FILE" | awk '{print $1}')"
if [[ "$actual_hash" != "${DR_EXPECTED_SHA256,,}" ]]; then
  echo 'Failback dump SHA-256 mismatch.' >&2
  exit 1
fi

audit_core_database() {
  local target="$1"
  local sensitive_rows
  sensitive_rows="$(sudo -u postgres psql -X -At -v ON_ERROR_STOP=1 -d "$target" <<'SQL'
WITH credential_keys AS (
  SELECT DISTINCT a.id,
    replace(replace(lower(trim(k.key)), '-', '_'), '.', '_') AS normalized
  FROM accounts a
  CROSS JOIN LATERAL jsonb_object_keys(COALESCE(a.credentials, '{}'::jsonb)) AS k(key)
  WHERE a.deleted_at IS NULL AND a.type IN ('oauth', 'setup-token')
)
SELECT count(DISTINCT id)
FROM credential_keys
WHERE normalized IN ('access_token','refresh_token','id_token','oauth_token','session_token','session_key','cookie','cookies','client_secret','authorization','authorization_header')
   OR normalized LIKE '%\_secret' ESCAPE '\'
   OR (normalized LIKE '%token%' AND normalized NOT IN ('token_type','oauth_type','token_version','_token_version','oauth_token_version'));
SQL
)"
  printf 'candidate_oauth_sensitive_rows=%s\n' "$sensitive_rows"
  [[ "$sensitive_rows" == '0' ]]

  DATABASE_DBNAME="$target" \
    OAUTH_VAULT_MODE=external \
    OAUTH_VAULT_ALLOW_LEGACY_FALLBACK=false \
    "$app_binary" --audit-oauth-vault 2>&1 | tee "/tmp/anl-oauth-vault-audit-${target}.log"
  grep -q 'sensitive_rows=0' "/tmp/anl-oauth-vault-audit-${target}.log"
  grep -q 'missing_vault_entries=0' "/tmp/anl-oauth-vault-audit-${target}.log"
  rm -f "/tmp/anl-oauth-vault-audit-${target}.log"
}

prepare_candidate() {
  sudo -u postgres psql -X -v ON_ERROR_STOP=1 -d postgres \
    -v candidate="$candidate_name" \
    -v owner="$database_owner" <<'SQL'
SELECT format('DROP DATABASE IF EXISTS %I WITH (FORCE)', :'candidate') \gexec
SELECT format('CREATE DATABASE %I OWNER %I', :'candidate', :'owner') \gexec
SQL
  sudo -u postgres pg_restore --no-owner --no-privileges --role="$database_owner" --dbname="$candidate_name" "$DR_DUMP_FILE"
  audit_core_database "$candidate_name"
  sudo -u postgres psql -X -At -v ON_ERROR_STOP=1 -d "$candidate_name" <<'SQL'
SELECT 'users=' || count(*) FROM users WHERE deleted_at IS NULL;
SELECT 'api_keys=' || count(*) FROM api_keys WHERE deleted_at IS NULL;
SELECT 'accounts=' || count(*) FROM accounts WHERE deleted_at IS NULL;
SELECT 'pending_payments=' || count(*) FROM payment_orders WHERE status = 'pending';
SQL
  echo 'candidate_ready=true'
}

activate_candidate() {
  if [[ "${DR_ACTIVATION_CONFIRMATION:-}" != 'DOMESTIC_STOPPED_AND_CANDIDATE_VERIFIED' ]]; then
    echo 'Set DR_ACTIVATION_CONFIRMATION=DOMESTIC_STOPPED_AND_CANDIDATE_VERIFIED before activation.' >&2
    exit 2
  fi
  audit_core_database "$candidate_name"
  local backup_name
  backup_name="${database_name}_before_failback_$(date -u +%Y%m%dT%H%M%SZ)"
  systemctl stop "$app_service"
  sudo -u postgres psql -X -v ON_ERROR_STOP=1 -d postgres \
    -v current="$database_name" \
    -v candidate="$candidate_name" \
    -v backup="$backup_name" <<'SQL'
SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname IN (:'current', :'candidate') AND pid <> pg_backend_pid();
SELECT format('ALTER DATABASE %I RENAME TO %I', :'current', :'backup') \gexec
SELECT format('ALTER DATABASE %I RENAME TO %I', :'candidate', :'current') \gexec
SQL
  if systemctl start "$app_service" && curl --fail --silent --show-error --max-time 30 http://127.0.0.1:8080/health >/dev/null; then
    printf 'old_core_database=%s\n' "$backup_name"
    echo 'failback_activation=true'
    return
  fi

  systemctl stop "$app_service" || true
  sudo -u postgres psql -X -v ON_ERROR_STOP=1 -d postgres \
    -v current="$database_name" \
    -v backup="$backup_name" \
    -v failed="${candidate_name}_failed" <<'SQL'
SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = :'current' AND pid <> pg_backend_pid();
SELECT format('ALTER DATABASE %I RENAME TO %I', :'current', :'failed') \gexec
SELECT format('ALTER DATABASE %I RENAME TO %I', :'backup', :'current') \gexec
SQL
  systemctl start "$app_service"
  echo 'Candidate activation failed and the previous US core database was restored.' >&2
  exit 1
}

case "$mode" in
  prepare) prepare_candidate ;;
  activate) activate_candidate ;;
  *) echo 'Usage: Restore-AnlDrCore.sh [prepare|activate]' >&2; exit 2 ;;
esac
