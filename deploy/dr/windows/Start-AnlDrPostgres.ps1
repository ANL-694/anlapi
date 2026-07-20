param(
  [string]$AppRoot = 'D:\anl-api',
  [int]$Port = 5432
)

$ErrorActionPreference = 'Stop'
$postgresRoot = Join-Path $AppRoot 'runtime\postgres'
$postgresData = Join-Path $AppRoot 'runtime\postgres-data'
$logsRoot = Join-Path $AppRoot 'data\logs'
$pgCtl = Join-Path $postgresRoot 'bin\pg_ctl.exe'

foreach ($required in @($pgCtl, $postgresData)) {
  if (-not (Test-Path -LiteralPath $required)) { throw ('Required path is missing: ' + $required) }
}
New-Item -ItemType Directory -Path $logsRoot -Force | Out-Null

$listener = @(Get-NetTCPConnection -LocalPort $Port -State Listen -ErrorAction SilentlyContinue)
if ($listener.Count -gt 0) {
  $unexpected = $listener | Where-Object {
    $process = Get-CimInstance Win32_Process -Filter ('ProcessId=' + $_.OwningProcess) -ErrorAction SilentlyContinue
    -not $process.ExecutablePath -or -not $process.ExecutablePath.StartsWith($postgresRoot, [StringComparison]::OrdinalIgnoreCase)
  }
  if ($unexpected) { throw ('PostgreSQL port is occupied by an unexpected process: ' + $Port) }
  Write-Output 'ANL_DR_POSTGRES_ALREADY_RUNNING=true'
  exit 0
}

$pidFile = Join-Path $postgresData 'postmaster.pid'
if (Test-Path -LiteralPath $pidFile) {
  $pidText = (Get-Content -LiteralPath $pidFile -Encoding ascii | Select-Object -First 1).Trim()
  $existingPid = 0
  if ([int]::TryParse($pidText, [ref]$existingPid) -and $existingPid -gt 0) {
    $existingProcess = Get-CimInstance Win32_Process -Filter ('ProcessId=' + $existingPid) -ErrorAction SilentlyContinue
    if ($existingProcess -and $existingProcess.Name -ieq 'postgres.exe') {
      throw ('PostgreSQL process exists without a listener: ' + $existingPid)
    }
  }
  Remove-Item -LiteralPath $pidFile -Force
  Write-Output 'ANL_DR_POSTGRES_STALE_PID_REMOVED=true'
}

& $pgCtl -D $postgresData -l (Join-Path $logsRoot 'postgres.log') -w -t 60 start | Out-Null
if ($LASTEXITCODE -ne 0) { throw 'Unable to start domestic PostgreSQL.' }
if (-not (Test-NetConnection 127.0.0.1 -Port $Port -InformationLevel Quiet -WarningAction SilentlyContinue)) {
  throw 'Domestic PostgreSQL did not become ready.'
}
Write-Output 'ANL_DR_POSTGRES_STARTED=true'
