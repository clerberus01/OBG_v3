param(
  [string]$PackageDir = "dist\omni-bot-go-2.0.0",
  [string]$Addr = "127.0.0.1:8099",
  [int]$TimeoutSeconds = 20
)

$ErrorActionPreference = "Stop"

$root = Resolve-Path (Join-Path $PSScriptRoot "..")
if (-not [System.IO.Path]::IsPathRooted($PackageDir)) {
  $PackageDir = Join-Path $root $PackageDir
}
$PackageDir = (Resolve-Path $PackageDir).Path
$runScript = Join-Path $PackageDir "run.ps1"
$binary = Join-Path $PackageDir "omni-bot-go.exe"

if (-not (Test-Path -LiteralPath $runScript)) {
  throw "run.ps1 nao encontrado no pacote: $PackageDir"
}
if (-not (Test-Path -LiteralPath $binary)) {
  throw "binario nao encontrado no pacote: $PackageDir"
}

$process = Start-Process -FilePath "powershell.exe" `
  -ArgumentList @("-NoProfile", "-ExecutionPolicy", "Bypass", "-File", $runScript, "-Addr", $Addr) `
  -WorkingDirectory $PackageDir `
  -WindowStyle Hidden `
  -PassThru

try {
  $deadline = (Get-Date).AddSeconds($TimeoutSeconds)
  $health = $null
  while ((Get-Date) -lt $deadline) {
    Start-Sleep -Milliseconds 500
    try {
      $health = Invoke-RestMethod -Uri "http://$Addr/api/health" -TimeoutSec 2
      break
    } catch {
      if ($process.HasExited) {
        throw "processo do pacote encerrou antes do health check"
      }
    }
  }
  if ($null -eq $health) {
    throw "dashboard do pacote nao respondeu em $TimeoutSeconds segundo(s)"
  }

  $engine = Invoke-RestMethod -Uri "http://$Addr/api/engine" -TimeoutSec 2
  $snapshot = Invoke-RestMethod -Uri "http://$Addr/api/snapshot" -TimeoutSec 2

  if ($health.status -ne "ok") {
    throw "health inesperado: $($health.status)"
  }
  if ($engine.mode -ne "symbolic") {
    throw "engine nao esta em modo symbolic: $($engine.mode)"
  }
if ([int]$engine.pack_count -lt 7) {
  throw "packs iniciais nao carregados: $($engine.pack_count)"
}
  if ($snapshot.PSObject.Properties.Name -contains "model") {
    throw "snapshot nao deve expor chave model"
  }

  [ordered]@{
    status = "ok"
    package = "$PackageDir"
    health = $health.status
    engine_mode = $engine.mode
    pack_count = $engine.pack_count
    snapshot_has_model = $false
  } | ConvertTo-Json
} finally {
  $listeners = netstat -ano | Select-String ":$($Addr.Split(':')[-1])"
  foreach ($line in $listeners) {
    $parts = ($line.ToString() -split "\s+") | Where-Object { $_ -ne "" }
    if ($parts.Length -ge 5 -and $parts[1] -like "*:$($Addr.Split(':')[-1])" -and $parts[3] -eq "LISTENING") {
      Stop-Process -Id ([int]$parts[-1]) -Force -ErrorAction SilentlyContinue
    }
  }
  if ($process -and -not $process.HasExited) {
    Stop-Process -Id $process.Id -Force -ErrorAction SilentlyContinue
  }
}

exit 0
