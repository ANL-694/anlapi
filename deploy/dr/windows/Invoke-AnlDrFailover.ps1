param(
  [Parameter(Mandatory = $true)]
  [string]$PrimaryFenceConfirmation,
  [string]$AppRoot = 'D:\anl-api',
  [string]$DatabaseName = 'anl_api_dr',
  [int]$MaximumReplicationAgeSeconds = 120,
  [switch]$AllowStaleReplica
)

$ErrorActionPreference = 'Stop'
if ($PrimaryFenceConfirmation -ne 'PRIMARY_WRITES_FENCED') {
  throw 'Refusing failover: confirm the US application and all writers are fenced with PRIMARY_WRITES_FENCED.'
}

$envPath = Join-Path $AppRoot '.env'
$drRoot = Join-Path $AppRoot 'dr'
$windowsRoot = Join-Path $drRoot 'windows'
$psql = Join-Path $AppRoot 'runtime\postgres\bin\psql.exe'
$redisCli = Join-Path $AppRoot 'runtime\redis\redis-cli.exe'
$redisServer = Join-Path $AppRoot 'runtime\redis\redis-server.exe'
$redisConfig = Join-Path $AppRoot 'redis.conf'
$backupScript = Join-Path $windowsRoot 'Backup-AnlDr.ps1'
$sequenceSql = Join-Path $drRoot 'sql\sequence-calibration.sql'
$stateRoot = Join-Path $drRoot 'state'

function Read-EnvFile([string]$Path) {
  $values = @{}
  foreach ($line in [IO.File]::ReadAllLines($Path, [Text.Encoding]::UTF8)) {
    $trimmed = $line.Trim()
    if ($trimmed.Length -eq 0 -or $trimmed.StartsWith('#')) { continue }
    $separator = $trimmed.IndexOf('=')
    if ($separator -gt 0) { $values[$trimmed.Substring(0, $separator)] = $trimmed.Substring($separator + 1) }
  }
  return $values
}

foreach ($required in @($envPath, $psql, $redisCli, $redisServer, $redisConfig, $backupScript, $sequenceSql)) {
  if (-not (Test-Path -LiteralPath $required)) { throw ('Required path is missing: ' + $required) }
}
Stop-ScheduledTask -TaskName 'ANL-API' -ErrorAction SilentlyContinue
$values = Read-EnvFile $envPath
$env:PGPASSWORD = $values['DATABASE_PASSWORD']
try {
  $subscription = & $psql -h 127.0.0.1 -p 5432 -U anl_api -d $DatabaseName -At -F '|' -v ON_ERROR_STOP=1 -c (
    "SELECT CASE WHEN pid IS NULL THEN 'stopped' ELSE 'streaming' END, COALESCE(EXTRACT(EPOCH FROM (clock_timestamp() - last_msg_receipt_time))::bigint, -1), COALESCE(received_lsn::text, ''), COALESCE(latest_end_lsn::text, '') FROM pg_stat_subscription WHERE subname = 'anl_us_to_cn' AND relid IS NULL"
  )
  if ($LASTEXITCODE -ne 0) { throw 'Unable to read subscription state.' }
  $parts = $subscription -split '\|', 4
  $messageAge = if ($parts.Count -ge 2) { [int64]$parts[1] } else { -1 }
  if (-not $AllowStaleReplica -and ($parts.Count -lt 2 -or $parts[0] -ne 'streaming' -or $messageAge -lt 0 -or $messageAge -gt $MaximumReplicationAgeSeconds)) {
    throw ('Replica is not fresh enough for failover. State: ' + $subscription)
  }

  & $backupScript -AppRoot $AppRoot -DatabaseName $DatabaseName -MaximumReplicationAgeSeconds $MaximumReplicationAgeSeconds -AllowDisabledSubscription:$AllowStaleReplica
  if ($LASTEXITCODE -ne 0) { throw 'Pre-failover backup failed.' }

  & $psql -h 127.0.0.1 -p 5432 -U postgres -d $DatabaseName -v ON_ERROR_STOP=1 -c 'ALTER SUBSCRIPTION anl_us_to_cn DISABLE'
  if ($LASTEXITCODE -ne 0) { throw 'Unable to disable the logical subscription.' }
  Start-Sleep -Seconds 3
  & $psql -h 127.0.0.1 -p 5432 -U anl_api -d $DatabaseName -v ON_ERROR_STOP=1 -f $sequenceSql
  if ($LASTEXITCODE -ne 0) { throw 'Final sequence calibration failed.' }

  $sensitiveRows = & $psql -h 127.0.0.1 -p 5432 -U anl_api -d $DatabaseName -At -v ON_ERROR_STOP=1 -c (
    "SELECT count(*) FROM accounts WHERE deleted_at IS NULL AND type IN ('oauth','setup-token') AND (credentials ? 'access_token' OR credentials ? 'refresh_token' OR credentials ? 'id_token' OR credentials ? 'cookie')"
  )
  if ($LASTEXITCODE -ne 0 -or $sensitiveRows -ne '0') { throw 'OAuth secrets are present in the domestic core database.' }
  $pendingPayments = & $psql -h 127.0.0.1 -p 5432 -U anl_api -d $DatabaseName -At -v ON_ERROR_STOP=1 -c "SELECT count(*) FROM payment_orders WHERE status = 'pending'"
} finally {
  Remove-Item Env:PGPASSWORD -ErrorAction SilentlyContinue
}

$env:REDISCLI_AUTH = $values['REDIS_PASSWORD']
try {
  if (@(Get-NetTCPConnection -LocalPort 6379 -State Listen -ErrorAction SilentlyContinue).Count -eq 0) {
    Start-Process -FilePath $redisServer -ArgumentList $redisConfig -WorkingDirectory (Split-Path -Parent $redisServer) -WindowStyle Hidden | Out-Null
    $redisDeadline = (Get-Date).AddSeconds(30)
    do {
      $redisReady = Test-NetConnection 127.0.0.1 -Port 6379 -InformationLevel Quiet -WarningAction SilentlyContinue
      if (-not $redisReady) { Start-Sleep -Seconds 1 }
    } until ($redisReady -or (Get-Date) -ge $redisDeadline)
    if (-not $redisReady) { throw 'Domestic Redis did not become ready.' }
  }
  & $redisCli -h 127.0.0.1 -p 6379 -n ([int]$values['REDIS_DB']) FLUSHDB | Out-Null
  if ($LASTEXITCODE -ne 0) { throw 'Unable to clear the domestic Redis database.' }
} finally {
  Remove-Item Env:REDISCLI_AUTH -ErrorAction SilentlyContinue
}

Start-ScheduledTask -TaskName 'ANL-API'
$healthDeadline = (Get-Date).AddSeconds(90)
do {
  try {
    $health = Invoke-WebRequest -UseBasicParsing -TimeoutSec 5 -Uri 'http://127.0.0.1:8822/health'
    $healthy = [int]$health.StatusCode -eq 200
  } catch {
    $healthy = $false
  }
  if (-not $healthy) { Start-Sleep -Seconds 3 }
} until ($healthy -or (Get-Date) -ge $healthDeadline)
if (-not $healthy) {
  Stop-ScheduledTask -TaskName 'ANL-API' -ErrorAction SilentlyContinue
  throw 'Domestic ANL API did not become healthy; it has been stopped again.'
}

New-Item -ItemType Directory -Path $stateRoot -Force | Out-Null
$statePath = Join-Path $stateRoot ('failover-' + (Get-Date).ToUniversalTime().ToString('yyyyMMddTHHmmssZ') + '.json')
$state = [ordered]@{
  activated_at_utc = (Get-Date).ToUniversalTime().ToString('o')
  database = $DatabaseName
  subscription_before_disable = [string]$subscription
  pending_payment_orders = [int]$pendingPayments
  primary_writes_fenced = $true
  oauth_mode = 'disabled'
} | ConvertTo-Json -Depth 3
[IO.File]::WriteAllText($statePath, $state + "`r`n", [Text.UTF8Encoding]::new($false))
Write-Output 'ANL_DR_FAILOVER_ACTIVE=true'
Write-Output ('PENDING_PAYMENT_ORDERS=' + $pendingPayments)
Write-Output ('FAILOVER_STATE=' + $statePath)
