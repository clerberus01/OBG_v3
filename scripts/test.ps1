$ErrorActionPreference = "Stop"

$root = Resolve-Path (Join-Path $PSScriptRoot "..")
$cache = Join-Path $root ".gocache"
$tmp = Join-Path $root "tmp\go-tmp"

New-Item -ItemType Directory -Force -Path $cache | Out-Null
New-Item -ItemType Directory -Force -Path $tmp | Out-Null

$env:CGO_ENABLED = "0"
$env:GOCACHE = $cache
$env:GOTMPDIR = $tmp

go test ./...
