param(
  [string]$Executable = ".\bin\omni-bot-go.exe",
  [string]$Addr = "127.0.0.1:8080",
  [string]$DB = "data/loja.db",
  [string]$Plugins = "plugins",
  [string]$Log = "logs/omni-bot-go.log",
  [int]$MaxRestarts = 10,
  [int]$BackoffSeconds = 5
)

$ErrorActionPreference = "Stop"

$root = Resolve-Path (Join-Path $PSScriptRoot "..")

function Resolve-FromRoot([string]$Path) {
  if ([System.IO.Path]::IsPathRooted($Path)) {
    return $Path
  }
  return Join-Path $root $Path
}

$Executable = Resolve-FromRoot $Executable
$DB = Resolve-FromRoot $DB
$Plugins = Resolve-FromRoot $Plugins
$Log = Resolve-FromRoot $Log
$supervisorLog = Join-Path $root "logs\supervisor.log"

New-Item -ItemType Directory -Force -Path (Join-Path $root "data"),(Join-Path $root "logs"),(Join-Path $root "projects") | Out-Null
New-Item -ItemType Directory -Force -Path (Split-Path -Parent $DB) | Out-Null
New-Item -ItemType Directory -Force -Path (Split-Path -Parent $Log) | Out-Null
New-Item -ItemType Directory -Force -Path $Plugins | Out-Null

if (-not (Test-Path -LiteralPath $Executable)) {
  throw "Executavel nao encontrado: $Executable. Gere o binario manualmente quando quiser supervisionar."
}

$restartCount = 0

while ($restartCount -lt $MaxRestarts) {
  $startedAt = Get-Date
  Add-Content -LiteralPath $supervisorLog -Value "[$($startedAt.ToString('s'))] starting $Executable"

  $argsList = @("-addr", $Addr, "-db", $DB, "-plugins", $Plugins, "-log", $Log)
  & $Executable @argsList *>> $supervisorLog
  $exitCode = $LASTEXITCODE

  $finishedAt = Get-Date
  Add-Content -LiteralPath $supervisorLog -Value "[$($finishedAt.ToString('s'))] exited code=$exitCode"

  if ($exitCode -eq 0) {
    exit 0
  }

  $restartCount++
  Start-Sleep -Seconds ([Math]::Min($BackoffSeconds * $restartCount, 60))
}

throw "Limite de reinicios atingido: $MaxRestarts"
