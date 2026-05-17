$ErrorActionPreference = "Stop"

$root = Resolve-Path (Join-Path $PSScriptRoot "..")
$cache = Join-Path $root ".gocache"
$tmp = Join-Path $root "tmp\go-tmp"

function Resolve-GoCommand {
  $preferred = Get-Command go1.26.3 -ErrorAction SilentlyContinue
  if ($preferred) { return $preferred.Source }
  $sideBySide = Join-Path $env:USERPROFILE "go\bin\go1.26.3.exe"
  if (Test-Path -LiteralPath $sideBySide) { return $sideBySide }
  $serviceDesk = "C:\Users\servicedesk.br\go\bin\go1.26.3.exe"
  if (Test-Path -LiteralPath $serviceDesk) { return $serviceDesk }
  return "go"
}

New-Item -ItemType Directory -Force -Path $cache | Out-Null
New-Item -ItemType Directory -Force -Path $tmp | Out-Null

$env:CGO_ENABLED = "0"
$env:GOCACHE = $cache
$env:GOTMPDIR = $tmp
$go = Resolve-GoCommand

& $go test ./...
