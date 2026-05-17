param(
  [string]$OutputDir = "bin",
  [string]$Version = "dev",
  [string]$Commit = "local",
  [switch]$Strip
)

$ErrorActionPreference = "Stop"

$root = Resolve-Path (Join-Path $PSScriptRoot "..")
$out = Join-Path $root $OutputDir
$cache = Join-Path $root ".gocache"
$tmp = Join-Path $root "tmp\go-tmp"
$buildTime = (Get-Date).ToUniversalTime().ToString("yyyy-MM-ddTHH:mm:ssZ")

function Resolve-GoCommand {
  $preferred = Get-Command go1.26.3 -ErrorAction SilentlyContinue
  if ($preferred) { return $preferred.Source }
  $sideBySide = Join-Path $env:USERPROFILE "go\bin\go1.26.3.exe"
  if (Test-Path -LiteralPath $sideBySide) { return $sideBySide }
  $serviceDesk = "C:\Users\servicedesk.br\go\bin\go1.26.3.exe"
  if (Test-Path -LiteralPath $serviceDesk) { return $serviceDesk }
  return "go"
}

New-Item -ItemType Directory -Force -Path $out | Out-Null
New-Item -ItemType Directory -Force -Path $cache | Out-Null
New-Item -ItemType Directory -Force -Path $tmp | Out-Null

$env:CGO_ENABLED = "0"
$env:GOCACHE = $cache
$env:GOTMPDIR = $tmp
$go = Resolve-GoCommand

$target = Join-Path $out "omni-bot-go.exe"
$ldflags = "-X main.version=$Version -X main.commit=$Commit -X main.buildTime=$buildTime"
if ($Strip) {
  $ldflags = "-s -w $ldflags"
}

if (Test-Path -LiteralPath $target) {
  Remove-Item -LiteralPath $target -Force
}

& $go build -ldflags $ldflags -o $target ./cmd
Write-Host "build ok: $target"
