param(
  [switch]$IncludeRuntime
)

$ErrorActionPreference = "Stop"

$root = Resolve-Path (Join-Path $PSScriptRoot "..")
$targets = @("bin", "dist", ".gocache", "tmp")
if ($IncludeRuntime) {
  $targets += @("data", "logs")
}

foreach ($name in $targets) {
  $path = Join-Path $root $name
  $resolvedRoot = [System.IO.Path]::GetFullPath($root)
  $resolvedPath = [System.IO.Path]::GetFullPath($path)
  if ($resolvedPath.StartsWith($resolvedRoot) -and (Test-Path $resolvedPath)) {
    Remove-Item -LiteralPath $resolvedPath -Recurse -Force
    Write-Host "removed: $resolvedPath"
  }
}
