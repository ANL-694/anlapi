param(
  [Parameter(Mandatory = $true)]
  [string]$DomesticFenceConfirmation,
  [string]$AppRoot = 'D:\anlapi',
  [string]$DatabaseName = 'anlapi_dr',
  [string]$ExportRoot = 'D:\anlapi-dr-failback'
)

$ErrorActionPreference = 'Stop'
if ($DomesticFenceConfirmation -ne 'DOMESTIC_WRITES_FENCED') {
  throw 'Refusing export until the domestic entry and all writers are fenced.'
}

$envPath = Join-Path $AppRoot '.env'
$pgDump = Join-Path $AppRoot 'runtime\postgres\bin\pg_dump.exe'
$psql = Join-Path $AppRoot 'runtime\postgres\bin\psql.exe'
$auditSql = Join-Path $AppRoot 'dr\sql\oauth-isolation-audit.sql'
$partial = $null

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

foreach ($appTaskName in @('ANLAPI', 'ANL-API')) {
  Stop-ScheduledTask -TaskName $appTaskName -ErrorAction SilentlyContinue
}
Start-Sleep -Seconds 10
$values = Read-EnvFile $envPath
$env:PGPASSWORD = $values['DATABASE_PASSWORD']
try {
  $audit = & $psql -h 127.0.0.1 -p 5432 -U ikik_api -d $DatabaseName -At -v ON_ERROR_STOP=1 -f $auditSql
  if ($LASTEXITCODE -ne 0 -or $audit -notcontains 'oauth_sensitive_rows=0') { throw 'OAuth isolation audit failed.' }
  New-Item -ItemType Directory -Path $ExportRoot -Force | Out-Null
  $timestamp = (Get-Date).ToUniversalTime().ToString('yyyyMMddTHHmmssZ')
  $partial = Join-Path $ExportRoot ('anl-core-failback-' + $timestamp + '.dump.partial')
  $final = Join-Path $ExportRoot ('anl-core-failback-' + $timestamp + '.dump')
  $previousErrorActionPreference = $ErrorActionPreference
  $ErrorActionPreference = 'Continue'
  & $pgDump -h 127.0.0.1 -p 5432 -U ikik_api -d $DatabaseName --format=custom --no-owner --no-privileges --file=$partial 2>&1 | Out-Null
  $dumpExitCode = $LASTEXITCODE
  $ErrorActionPreference = $previousErrorActionPreference
  if ($dumpExitCode -ne 0) { throw 'Final failback pg_dump failed.' }
  $hash = (Get-FileHash -LiteralPath $partial -Algorithm SHA256).Hash.ToLowerInvariant()
  Move-Item -LiteralPath $partial -Destination $final
  $manifest = [ordered]@{
    created_at_utc = (Get-Date).ToUniversalTime().ToString('o')
    database = $DatabaseName
    bytes = (Get-Item -LiteralPath $final).Length
    sha256 = $hash
    oauth_isolation = [string[]]$audit
    domestic_writes_fenced = $true
  } | ConvertTo-Json -Depth 4
  [IO.File]::WriteAllText($final + '.json', $manifest + "`r`n", [Text.UTF8Encoding]::new($false))
  Write-Output ('FAILBACK_FILE=' + $final)
  Write-Output ('FAILBACK_SHA256=' + $hash)
} finally {
  Remove-Item Env:PGPASSWORD -ErrorAction SilentlyContinue
  if ($partial -and (Test-Path -LiteralPath $partial)) { Remove-Item -LiteralPath $partial -Force }
}
