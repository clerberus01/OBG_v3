param(
  [string]$Addr = "127.0.0.1:8080",
  [string]$DB = "data/loja.db",
  [string]$Commands = "",
  [string]$Plugins = "plugins",
  [string]$Log = "logs/omni-bot-go.log",
  [switch]$EnableGoValidation
)

$ErrorActionPreference = "Stop"

$root = Resolve-Path (Join-Path $PSScriptRoot "..")

function Resolve-FromRoot([string]$Path) {
  if ([System.IO.Path]::IsPathRooted($Path)) {
    return $Path
  }
  return Join-Path $root $Path
}

$cache = Join-Path $root ".gocache"
$tmp = Join-Path $root "tmp\go-tmp"
$dbPath = Resolve-FromRoot $DB
$pluginsPath = Resolve-FromRoot $Plugins
$logPath = Resolve-FromRoot $Log

New-Item -ItemType Directory -Force -Path (Split-Path -Parent $dbPath) | Out-Null
New-Item -ItemType Directory -Force -Path (Split-Path -Parent $logPath) | Out-Null
New-Item -ItemType Directory -Force -Path $pluginsPath | Out-Null
New-Item -ItemType Directory -Force -Path $cache | Out-Null
New-Item -ItemType Directory -Force -Path $tmp | Out-Null

$env:CGO_ENABLED = "0"
$env:GOCACHE = $cache
$env:GOTMPDIR = $tmp
if ($EnableGoValidation) {
  $env:OBG_ALLOW_GO_VALIDATION = "1"
} else {
  Remove-Item Env:OBG_ALLOW_GO_VALIDATION -ErrorAction SilentlyContinue
}

$argsList = @((Join-Path $root "cmd"), "-addr", $Addr, "-db", $dbPath, "-plugins", $pluginsPath, "-log", $logPath)
if ($Commands.Trim() -ne "") {
  $commandsPath = $Commands
  if (-not [System.IO.Path]::IsPathRooted($commandsPath)) {
    $commandsPath = Resolve-FromRoot $commandsPath
  }
  $argsList += @("-commands", $commandsPath)
}

go run @argsList
