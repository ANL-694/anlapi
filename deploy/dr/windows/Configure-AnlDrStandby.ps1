param(
  [string]$AppRoot = 'D:\anl-api',
  [int]$BackupIntervalHours = 1,
  [int]$BackupRetentionCount = 72
)

$ErrorActionPreference = 'Stop'
$drRoot = Join-Path $AppRoot 'dr'
$windowsRoot = Join-Path $drRoot 'windows'
$supervisor = Join-Path $AppRoot 'supervise-anl-api.ps1'
$postgresScript = Join-Path $windowsRoot 'Start-AnlDrPostgres.ps1'
$backupScript = Join-Path $windowsRoot 'Backup-AnlDr.ps1'
$redisRoot = Join-Path $AppRoot 'runtime\redis'
$appBinary = Join-Path $AppRoot 'anlapi.exe'

foreach ($required in @($supervisor, $postgresScript, $backupScript, $appBinary)) {
  if (-not (Test-Path -LiteralPath $required)) { throw ('Required path is missing: ' + $required) }
}
if ($BackupIntervalHours -lt 1) { throw 'BackupIntervalHours must be at least 1.' }

Stop-ScheduledTask -TaskName 'ANL-API' -ErrorAction SilentlyContinue
Get-CimInstance Win32_Process -ErrorAction SilentlyContinue |
  Where-Object {
    ($_.ExecutablePath -and $_.ExecutablePath.Equals($appBinary, [StringComparison]::OrdinalIgnoreCase)) -or
    ($_.ExecutablePath -and $_.ExecutablePath.StartsWith($redisRoot, [StringComparison]::OrdinalIgnoreCase))
  } |
  ForEach-Object { Stop-Process -Id $_.ProcessId -Force -ErrorAction SilentlyContinue }

$principal = New-ScheduledTaskPrincipal -UserId 'SYSTEM' -LogonType ServiceAccount -RunLevel Highest
$persistentSettings = New-ScheduledTaskSettingsSet -RestartCount 999 -RestartInterval (New-TimeSpan -Minutes 1) -ExecutionTimeLimit ([TimeSpan]::Zero) -MultipleInstances IgnoreNew

$appAction = New-ScheduledTaskAction -Execute 'powershell.exe' -Argument (
  '-NoProfile -ExecutionPolicy Bypass -File "{0}"' -f $supervisor
)
Unregister-ScheduledTask -TaskName 'ANL-API' -Confirm:$false -ErrorAction SilentlyContinue
Register-ScheduledTask -TaskName 'ANL-API' -Action $appAction -Principal $principal -Settings $persistentSettings -Description 'ANL API manual DR application task; no automatic trigger' | Out-Null

$postgresAction = New-ScheduledTaskAction -Execute 'powershell.exe' -Argument (
  '-NoProfile -ExecutionPolicy Bypass -File "{0}" -AppRoot "{1}"' -f $postgresScript, $AppRoot
)
$startupTrigger = New-ScheduledTaskTrigger -AtStartup
Register-ScheduledTask -TaskName 'ANL-API-DR-Postgres' -Action $postgresAction -Trigger $startupTrigger -Principal $principal -Settings $persistentSettings -Description 'Keep the domestic ANL DR PostgreSQL subscriber available' -Force | Out-Null
Start-ScheduledTask -TaskName 'ANL-API-DR-Postgres'

$backupAction = New-ScheduledTaskAction -Execute 'powershell.exe' -Argument (
  '-NoProfile -ExecutionPolicy Bypass -File "{0}" -AppRoot "{1}" -RetentionCount {2}' -f $backupScript, $AppRoot, $BackupRetentionCount
)
$backupTrigger = New-ScheduledTaskTrigger -Once -At (Get-Date).AddMinutes(5) -RepetitionInterval (New-TimeSpan -Hours $BackupIntervalHours) -RepetitionDuration (New-TimeSpan -Days 3650)
$backupSettings = New-ScheduledTaskSettingsSet -RestartCount 3 -RestartInterval (New-TimeSpan -Minutes 5) -ExecutionTimeLimit (New-TimeSpan -Minutes 45) -MultipleInstances IgnoreNew -StartWhenAvailable
Register-ScheduledTask -TaskName 'ANL-API-DR-Backup' -Action $backupAction -Trigger $backupTrigger -Principal $principal -Settings $backupSettings -Description 'Hourly ANL API domestic core database backup' -Force | Out-Null

$appTask = Get-ScheduledTask -TaskName 'ANL-API'
if ($appTask.State -ne 'Ready') { throw ('ANL-API task is not in standby state: ' + $appTask.State) }
if (@(Get-NetTCPConnection -LocalPort 8822 -State Listen -ErrorAction SilentlyContinue).Count -ne 0) {
  throw 'Domestic ANL API port must remain stopped after standby configuration.'
}
Write-Output 'ANL_DR_STANDBY_TASKS_READY=true'
Write-Output ('ANL_API_TASK_STATE=' + $appTask.State)
