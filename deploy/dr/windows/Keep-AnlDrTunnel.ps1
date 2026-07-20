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

while ($true) {
  ('{0:o} starting SSH tunnel' -f (Get-Date)) | Add-Content -LiteralPath $LogFile -Encoding utf8
  & ssh.exe `
    -NT `
    -p $PrimaryPort `
    -i $IdentityFile `
    -o BatchMode=yes `
    -o ExitOnForwardFailure=yes `
    -o ServerAliveInterval=30 `
    -o ServerAliveCountMax=3 `
    -o StrictHostKeyChecking=yes `
    -o ('UserKnownHostsFile=' + $KnownHostsFile) `
    -L ('127.0.0.1:{0}:127.0.0.1:5432' -f $LocalPort) `
    ('{0}@{1}' -f $TunnelUser, $PrimaryHost) 2>&1 |
    ForEach-Object { ('{0:o} {1}' -f (Get-Date), $_) } |
    Add-Content -LiteralPath $LogFile -Encoding utf8
  ('{0:o} SSH tunnel exited with code {1}; retrying' -f (Get-Date), $LASTEXITCODE) |
    Add-Content -LiteralPath $LogFile -Encoding utf8
  Start-Sleep -Seconds 10
}
