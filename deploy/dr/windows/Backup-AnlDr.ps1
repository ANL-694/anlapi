param(
  [string]$AppRoot = 'D:\anl-api',
  [string]$DatabaseName = 'anlapi_dr',
  [string]$BackupRoot = 'D:\anl-api-dr-backups',
  [int]$MaximumReplicationAgeSeconds = 120,
  [int]$RetentionCount = 72,
  [switch]$AllowDisabledSubscription
)

$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'
$envPath = Join-Path $AppRoot '.env'
$pgBin = Join-Path $AppRoot 'runtime\postgres\bin'
$psql = Join-Path $pgBin 'psql.exe'
$pgDump = Join-Path $pgBin 'pg_dump.exe'
$drRoot = Join-Path $AppRoot 'dr'
$auditSql = Join-Path $drRoot 'sql\oauth-isolation-audit.sql'
$sequenceSql = Join-Path $drRoot 'sql\sequence-calibration.sql'
$partialPath = $null

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

foreach ($required in @($envPath, $psql, $pgDump, $auditSql, $sequenceSql)) {
  if (-not (Test-Path -LiteralPath $required)) { throw ('Required path is missing: ' + $required) }
}
$values = Read-EnvFile $envPath
$env:PGPASSWORD = $values['DATABASE_PASSWORD']
if ([string]::IsNullOrWhiteSpace($env:PGPASSWORD)) { throw 'DATABASE_PASSWORD is missing.' }

try {
  $subscription = & $psql -h 127.0.0.1 -p 5432 -U ikik_api -d $DatabaseName -At -F '|' -v ON_ERROR_STOP=1 -c (
    "SELECT CASE WHEN pid IS NULL THEN 'stopped' ELSE 'streaming' END, COALESCE(EXTRACT(EPOCH FROM (clock_timestamp() - last_msg_receipt_time))::bigint, -1) FROM pg_stat_subscription WHERE subname = 'anl_us_to_cn' AND relid IS NULL"
  )
  if ($LASTEXITCODE -ne 0) { throw 'Unable to read subscription health.' }
  if (-not $AllowDisabledSubscription) {
    if ([string]::IsNullOrWhiteSpace($subscription)) { throw 'Logical subscription worker is not running.' }
    $parts = $subscription -split '\|', 2
    if ($parts[0] -ne 'streaming') { throw ('Logical subscription status is not streaming: ' + $parts[0]) }
    if ([int64]$parts[1] -lt 0 -or [int64]$parts[1] -gt $MaximumReplicationAgeSeconds) {
      throw ('Logical subscription message age is too high: ' + $parts[1] + ' seconds')
    }
    $notReady = & $psql -h 127.0.0.1 -p 5432 -U ikik_api -d $DatabaseName -At -v ON_ERROR_STOP=1 -c (
      "SELECT count(*) FROM pg_subscription_rel WHERE srsubstate <> 'r'"
    )
    if ($LASTEXITCODE -ne 0 -or $notReady -ne '0') { throw ('Subscription tables not ready: ' + $notReady) }
  }

  $audit = & $psql -h 127.0.0.1 -p 5432 -U ikik_api -d $DatabaseName -At -v ON_ERROR_STOP=1 -f $auditSql
  if ($LASTEXITCODE -ne 0 -or $audit -notcontains 'oauth_sensitive_rows=0') {
    throw 'OAuth isolation audit failed; refusing to create a domestic backup.'
  }

  & $psql -h 127.0.0.1 -p 5432 -U ikik_api -d $DatabaseName -v ON_ERROR_STOP=1 -f $sequenceSql
  if ($LASTEXITCODE -ne 0) { throw 'Sequence calibration failed.' }

  New-Item -ItemType Directory -Path $BackupRoot -Force | Out-Null
  $timestamp = (Get-Date).ToUniversalTime().ToString('yyyyMMddTHHmmssZ')
  $partialPath = Join-Path $BackupRoot ('anl-core-' + $timestamp + '.dump.partial')
  $finalPath = Join-Path $BackupRoot ('anl-core-' + $timestamp + '.dump')
  $manifestPath = $finalPath + '.json'
  $previousErrorActionPreference = $ErrorActionPreference
  $ErrorActionPreference = 'Continue'
  & $pgDump -h 127.0.0.1 -p 5432 -U ikik_api -d $DatabaseName --format=custom --no-owner --no-privileges --file=$partialPath 2>&1 | Out-Null
  $dumpExitCode = $LASTEXITCODE
  $ErrorActionPreference = $previousErrorActionPreference
  if ($dumpExitCode -ne 0) { throw 'Domestic pg_dump failed.' }
  $hash = (Get-FileHash -LiteralPath $partialPath -Algorithm SHA256).Hash.ToLowerInvariant()
  Move-Item -LiteralPath $partialPath -Destination $finalPath
  $manifest = [ordered]@{
    created_at_utc = (Get-Date).ToUniversalTime().ToString('o')
    database = $DatabaseName
    bytes = (Get-Item -LiteralPath $finalPath).Length
    sha256 = $hash
    subscription = [string]$subscription
    oauth_isolation = [string[]]$audit
  } | ConvertTo-Json -Depth 4
  [IO.File]::WriteAllText($manifestPath, $manifest + "`r`n", [Text.UTF8Encoding]::new($false))
  (Get-Item -LiteralPath $finalPath).IsReadOnly = $true
  (Get-Item -LiteralPath $manifestPath).IsReadOnly = $true

  $oldBackups = Get-ChildItem -LiteralPath $BackupRoot -File -Filter 'anl-core-*.dump' |
    Sort-Object LastWriteTimeUtc -Descending |
    Select-Object -Skip $RetentionCount
  foreach ($oldBackup in $oldBackups) {
    $oldBackup.IsReadOnly = $false
    Remove-Item -LiteralPath $oldBackup.FullName -Force
    $oldManifest = $oldBackup.FullName + '.json'
    if (Test-Path -LiteralPath $oldManifest) {
      (Get-Item -LiteralPath $oldManifest).IsReadOnly = $false
      Remove-Item -LiteralPath $oldManifest -Force
    }
  }
  Write-Output ('BACKUP_FILE=' + $finalPath)
  Write-Output ('BACKUP_SHA256=' + $hash)
} finally {
  Remove-Item Env:PGPASSWORD -ErrorAction SilentlyContinue
  if ($partialPath -and (Test-Path -LiteralPath $partialPath -ErrorAction SilentlyContinue)) {
    Remove-Item -LiteralPath $partialPath -Force -ErrorAction SilentlyContinue
  }
}
