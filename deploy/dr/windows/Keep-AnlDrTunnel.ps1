param(
  [string]$PrimaryHost = '23.149.60.41',
  [int]$PrimaryPort = 22,
  [string]$TunnelUser = 'anl-dr-tunnel',
  [int]$LocalPort = 55432,
  [string]$IdentityFile = 'D:\anl-api\dr\keys\anl-dr-tunnel-ed25519',
  [string]$KnownHostsFile = 'D:\anl-api\dr\keys\known_hosts',
  [string]$LogFile = 'D:\anl-api\dr\logs\ssh-tunnel.log'
)

$ErrorActionPreference = 'Stop'
$logDirectory = Split-Path -Parent $LogFile
New-Item -ItemType Directory -Path $logDirectory -Force | Out-Null
$sshPath = Join-Path $env:WINDIR 'System32\OpenSSH\ssh.exe'
if (-not (Test-Path -LiteralPath $sshPath)) {
  throw ('OpenSSH client is missing: ' + $sshPath)
}

$stdoutPath = $LogFile + '.stdout.tmp'
$stderrPath = $LogFile + '.stderr.tmp'

function Append-ProcessLog([string]$Path) {
  if (-not (Test-Path -LiteralPath $Path)) { return }
  Get-Content -LiteralPath $Path -Encoding UTF8 -ErrorAction SilentlyContinue |
    ForEach-Object { ('{0:o} {1}' -f (Get-Date), $_) } |
    Add-Content -LiteralPath $LogFile -Encoding utf8
  Remove-Item -LiteralPath $Path -Force -ErrorAction SilentlyContinue
}

while ($true) {
  ('{0:o} starting SSH tunnel' -f (Get-Date)) | Add-Content -LiteralPath $LogFile -Encoding utf8
  Remove-Item -LiteralPath $stdoutPath, $stderrPath -Force -ErrorAction SilentlyContinue

  $arguments = @(
    '-NT',
    '-p', [string]$PrimaryPort,
    '-i', $IdentityFile,
    '-o', 'BatchMode=yes',
    '-o', 'ExitOnForwardFailure=yes',
    '-o', 'ServerAliveInterval=30',
    '-o', 'ServerAliveCountMax=3',
    '-o', 'StrictHostKeyChecking=yes',
    '-o', ('UserKnownHostsFile=' + $KnownHostsFile),
    '-L', ('127.0.0.1:{0}:127.0.0.1:5432' -f $LocalPort),
    ('{0}@{1}' -f $TunnelUser, $PrimaryHost)
  )

  $exitCode = 1
  try {
    $process = Start-Process `
      -FilePath $sshPath `
      -ArgumentList $arguments `
      -PassThru `
      -WindowStyle Hidden `
      -RedirectStandardOutput $stdoutPath `
      -RedirectStandardError $stderrPath
    $process.WaitForExit()
    $exitCode = $process.ExitCode
  } catch {
    ('{0:o} unable to run SSH tunnel: {1}' -f (Get-Date), $_.Exception.Message) |
      Add-Content -LiteralPath $LogFile -Encoding utf8
  } finally {
    Append-ProcessLog $stdoutPath
    Append-ProcessLog $stderrPath
  }

  ('{0:o} SSH tunnel exited with code {1}; retrying' -f (Get-Date), $exitCode) |
    Add-Content -LiteralPath $LogFile -Encoding utf8
  Start-Sleep -Seconds 10
}
