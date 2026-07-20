param(
  [string]$AppRoot = 'D:\anl-api',
  [string]$PrimaryHost = '23.149.60.41',
  [int]$PrimarySshPort = 22,
  [string]$TunnelUser = 'anl-dr-tunnel',
  [string]$ExpectedEd25519Fingerprint = 'SHA256:tB21a5escQPgvx4bwdMigwtgIbhGAE2wZxppCEzM3s4',
  [string]$ReplicationUser = 'anl_dr_replica',
  [securestring]$ReplicationPassword,
  [string]$StandbyDatabase = 'anlapi_dr',
  [int]$TunnelPort = 55432,
  [switch]$Reinitialize,
  [int]$InitialCopyTimeoutMinutes = 180
)

$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'
$drRoot = Join-Path $AppRoot 'dr'
$windowsRoot = Join-Path $drRoot 'windows'
$keyRoot = Join-Path $drRoot 'keys'
$logRoot = Join-Path $drRoot 'logs'
$envPath = Join-Path $AppRoot '.env'
$pgBin = Join-Path $AppRoot 'runtime\postgres\bin'
$psql = Join-Path $pgBin 'psql.exe'
$pgDump = Join-Path $pgBin 'pg_dump.exe'
$pgRestore = Join-Path $pgBin 'pg_restore.exe'
$identityFile = Join-Path $keyRoot 'anl-dr-tunnel-ed25519'
$knownHostsFile = Join-Path $keyRoot 'known_hosts'
$pinnedHostKeyFile = Join-Path $keyRoot 'primary-known_hosts'
$tunnelScript = Join-Path $windowsRoot 'Keep-AnlDrTunnel.ps1'
$standbyScript = Join-Path $windowsRoot 'Configure-AnlDrStandby.ps1'
$schemaDump = Join-Path $drRoot 'publisher-schema.dump'
$subscriptionName = 'anl_us_to_cn'

function Read-EnvFile([string]$Path) {
  $values = @{}
  foreach ($line in [IO.File]::ReadAllLines($Path, [Text.Encoding]::UTF8)) {
    $trimmed = $line.Trim()
    if ($trimmed.Length -eq 0 -or $trimmed.StartsWith('#')) { continue }
    $separator = $trimmed.IndexOf('=')
    if ($separator -gt 0) {
      $values[$trimmed.Substring(0, $separator)] = $trimmed.Substring($separator + 1)
    }
  }
  return $values
}

function Set-EnvValues([string]$Path, [hashtable]$Updates) {
  $lines = [Collections.Generic.List[string]]::new()
  $seen = @{}
  foreach ($line in [IO.File]::ReadAllLines($Path, [Text.Encoding]::UTF8)) {
    $separator = $line.IndexOf('=')
    if ($separator -gt 0) {
      $key = $line.Substring(0, $separator)
      if ($Updates.ContainsKey($key)) {
        $lines.Add($key + '=' + [string]$Updates[$key])
        $seen[$key] = $true
        continue
      }
    }
    $lines.Add($line)
  }
  foreach ($key in $Updates.Keys) {
    if (-not $seen.ContainsKey($key)) { $lines.Add($key + '=' + [string]$Updates[$key]) }
  }
  $backup = $Path + '.before-dr-' + (Get-Date -Format 'yyyyMMddTHHmmss')
  Copy-Item -LiteralPath $Path -Destination $backup
  $content = [string]::Join("`r`n", $lines) + "`r`n"
  [IO.File]::WriteAllText($Path, $content, [Text.UTF8Encoding]::new($false))
  return $backup
}

function ConvertTo-PlainText([securestring]$Value) {
  $pointer = [Runtime.InteropServices.Marshal]::SecureStringToBSTR($Value)
  try { return [Runtime.InteropServices.Marshal]::PtrToStringBSTR($pointer) }
  finally { [Runtime.InteropServices.Marshal]::ZeroFreeBSTR($pointer) }
}

function Escape-ConnInfoValue([string]$Value) {
  return "'" + $Value.Replace('\', '\\').Replace("'", "\'") + "'"
}

foreach ($required in @($envPath, $psql, $pgDump, $pgRestore, $tunnelScript, $standbyScript)) {
  if (-not (Test-Path -LiteralPath $required)) { throw ('Required path is missing: ' + $required) }
}
New-Item -ItemType Directory -Path $keyRoot, $logRoot -Force | Out-Null

if (-not (Test-Path -LiteralPath $identityFile)) {
  & ssh-keygen.exe -q -t ed25519 -N '' -C 'anl-dr-tunnel' -f $identityFile
  if ($LASTEXITCODE -ne 0) { throw 'Unable to generate the DR tunnel key.' }
  & icacls.exe $identityFile /inheritance:r /grant:r '*S-1-5-18:F' '*S-1-5-32-544:F' | Out-Null
}

$scannedKeys = @()
if (Test-Path -LiteralPath $pinnedHostKeyFile) {
  $scannedKeys = @(Get-Content -LiteralPath $pinnedHostKeyFile -Encoding utf8 | Where-Object {
    $_ -match ('^' + [regex]::Escape($PrimaryHost) + '\s+ssh-ed25519\s+[A-Za-z0-9+/=]+')
  })
} else {
  for ($attempt = 1; $attempt -le 10 -and $scannedKeys.Count -eq 0; $attempt += 1) {
    $scannedKeys = @(& ssh-keyscan.exe -T 10 -p $PrimarySshPort -t ed25519 $PrimaryHost 2>$null | Where-Object { $_ -match '(^|\s)ssh-ed25519\s+[A-Za-z0-9+/=]+' })
    if ($scannedKeys.Count -eq 0) { Start-Sleep -Seconds 2 }
  }
}
if (-not $scannedKeys) { throw 'Unable to scan the primary SSH host key.' }
$fingerprintOutput = $scannedKeys | & ssh-keygen.exe -lf - -E sha256
if ($fingerprintOutput -notmatch [regex]::Escape($ExpectedEd25519Fingerprint)) {
  throw 'Primary SSH host key fingerprint does not match the pinned value.'
}
[IO.File]::WriteAllLines($knownHostsFile, [string[]]$scannedKeys, [Text.UTF8Encoding]::new($false))

$publicKeyInstalledMarker = Join-Path $keyRoot 'public-key-installed'
if (-not (Test-Path -LiteralPath $publicKeyInstalledMarker)) {
  Write-Output ('TUNNEL_PUBLIC_KEY_FILE=' + $identityFile + '.pub')
  throw 'Install the public key on the primary tunnel account, then create keys\public-key-installed.'
}

$taskAction = New-ScheduledTaskAction -Execute 'powershell.exe' -Argument (
  '-NoProfile -ExecutionPolicy Bypass -File "{0}" -PrimaryHost "{1}" -PrimaryPort {2} -TunnelUser "{3}" -LocalPort {4} -IdentityFile "{5}" -KnownHostsFile "{6}"' -f
  $tunnelScript, $PrimaryHost, $PrimarySshPort, $TunnelUser, $TunnelPort, $identityFile, $knownHostsFile
)
$taskTrigger = New-ScheduledTaskTrigger -AtStartup
$taskSettings = New-ScheduledTaskSettingsSet -RestartCount 999 -RestartInterval (New-TimeSpan -Minutes 1) -ExecutionTimeLimit ([TimeSpan]::Zero)
Register-ScheduledTask -TaskName 'ANL-API-DR-Tunnel' -Action $taskAction -Trigger $taskTrigger -Settings $taskSettings -User 'SYSTEM' -RunLevel Highest -Force | Out-Null
Start-ScheduledTask -TaskName 'ANL-API-DR-Tunnel'

$deadline = (Get-Date).AddSeconds(60)
do {
  $tunnelReady = Test-NetConnection 127.0.0.1 -Port $TunnelPort -InformationLevel Quiet -WarningAction SilentlyContinue
  if (-not $tunnelReady) { Start-Sleep -Seconds 2 }
} until ($tunnelReady -or (Get-Date) -ge $deadline)
if (-not $tunnelReady) { throw 'The DR SSH tunnel did not become ready.' }

if ($null -eq $ReplicationPassword) { $ReplicationPassword = Read-Host 'Replication database password' -AsSecureString }
$replicationPasswordText = ConvertTo-PlainText $ReplicationPassword
$envValues = Read-EnvFile $envPath
$localPassword = $envValues['DATABASE_PASSWORD']
if ([string]::IsNullOrWhiteSpace($localPassword)) { throw 'DATABASE_PASSWORD is missing.' }

$env:PGPASSWORD = $localPassword
try {
  $localVersion = & $psql -h 127.0.0.1 -p 5432 -U postgres -d postgres -At -v ON_ERROR_STOP=1 -c 'SHOW server_version_num'
  if ($LASTEXITCODE -ne 0 -or [int]$localVersion -lt 140000) { throw 'Domestic PostgreSQL 14 or newer is required.' }

  $databaseExists = & $psql -h 127.0.0.1 -p 5432 -U postgres -d postgres -At -v ON_ERROR_STOP=1 -c (
    "SELECT count(*) FROM pg_database WHERE datname = '" + $StandbyDatabase.Replace("'", "''") + "'"
  )
  if ($databaseExists -eq '1' -and -not $Reinitialize) {
    throw ('Standby database already exists: ' + $StandbyDatabase + '. Use -Reinitialize explicitly.')
  }
  if ($databaseExists -eq '1') {
    & $psql -h 127.0.0.1 -p 5432 -U postgres -d postgres -v ON_ERROR_STOP=1 -c (
      'DROP DATABASE "' + $StandbyDatabase.Replace('"', '""') + '" WITH (FORCE)'
    )
    if ($LASTEXITCODE -ne 0) { throw 'Unable to drop the old standby database.' }
  }
  & $psql -h 127.0.0.1 -p 5432 -U postgres -d postgres -v ON_ERROR_STOP=1 -c (
    'CREATE DATABASE "' + $StandbyDatabase.Replace('"', '""') + '" OWNER ikik_api'
  )
  if ($LASTEXITCODE -ne 0) { throw 'Unable to create the standby database.' }
} finally {
  Remove-Item Env:PGPASSWORD -ErrorAction SilentlyContinue
}

$env:PGPASSWORD = $replicationPasswordText
try {
  & $pgDump -h 127.0.0.1 -p $TunnelPort -U $ReplicationUser -d ikik_api --schema-only --no-owner --no-privileges --format=custom --file=$schemaDump
  if ($LASTEXITCODE -ne 0) { throw 'Unable to export the primary schema.' }
} finally {
  Remove-Item Env:PGPASSWORD -ErrorAction SilentlyContinue
}

$env:PGPASSWORD = $localPassword
try {
  & $pgRestore -h 127.0.0.1 -p 5432 -U ikik_api -d $StandbyDatabase --no-owner --no-privileges $schemaDump
  if ($LASTEXITCODE -ne 0) { throw 'Unable to restore the standby schema.' }

  $connInfo = 'host=127.0.0.1 port=' + $TunnelPort + ' dbname=ikik_api user=' + $ReplicationUser + ' password=' + (Escape-ConnInfoValue $replicationPasswordText) + ' sslmode=disable'
  $createSubscriptionSql = "SELECT format('CREATE SUBSCRIPTION anl_us_to_cn CONNECTION %L PUBLICATION anl_core_publication WITH (copy_data=true, create_slot=true, enabled=true, slot_name=''anl_cn_subscription'')', :'conninfo') \gexec"
  $createSubscriptionSql | & $psql -h 127.0.0.1 -p 5432 -U postgres -d $StandbyDatabase -v ON_ERROR_STOP=1 -v ('conninfo=' + $connInfo) 2>$null
  if ($LASTEXITCODE -ne 0) { throw 'Unable to create the logical subscription.' }

  $copyDeadline = (Get-Date).AddMinutes($InitialCopyTimeoutMinutes)
  do {
    $remaining = & $psql -h 127.0.0.1 -p 5432 -U postgres -d $StandbyDatabase -At -v ON_ERROR_STOP=1 -c (
      "SELECT count(*) FROM pg_subscription_rel WHERE srsubstate <> 'r'"
    )
    if ($LASTEXITCODE -ne 0) { throw 'Unable to read initial copy status.' }
    Write-Output ('INITIAL_COPY_REMAINING=' + $remaining)
    if ($remaining -ne '0') { Start-Sleep -Seconds 15 }
  } until ($remaining -eq '0' -or (Get-Date) -ge $copyDeadline)
  if ($remaining -ne '0') { throw 'Initial logical copy did not finish before the timeout.' }
} finally {
  Remove-Item Env:PGPASSWORD -ErrorAction SilentlyContinue
  $replicationPasswordText = $null
}

Stop-ScheduledTask -TaskName 'ANL-API' -ErrorAction SilentlyContinue
$envBackup = Set-EnvValues $envPath @{
  DATABASE_DBNAME = $StandbyDatabase
  OAUTH_VAULT_MODE = 'disabled'
  OAUTH_VAULT_DSN = ''
  OAUTH_VAULT_ENCRYPTION_KEY = ''
  OAUTH_VAULT_ALLOW_LEGACY_FALLBACK = 'false'
}
Write-Output ('ENV_BACKUP=' + $envBackup)
& $standbyScript -AppRoot $AppRoot
if ($LASTEXITCODE -ne 0) { throw 'Unable to configure domestic standby tasks.' }
Write-Output 'ANL_DR_SUBSCRIBER_READY=true'
