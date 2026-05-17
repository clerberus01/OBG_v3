param(
  [string]$Version = "dev",
  [string]$Commit = "local",
  [string]$OutputDir = "dist"
)

$ErrorActionPreference = "Stop"

$root = Resolve-Path (Join-Path $PSScriptRoot "..")
$distRoot = Join-Path $root $OutputDir
$packageName = "omni-bot-go-$Version"
$packageDir = Join-Path $distRoot $packageName
$packageBuildRel = "tmp\package-bin"
$packageBuildDir = Join-Path $root $packageBuildRel

$resolvedRoot = [System.IO.Path]::GetFullPath($root)
$resolvedDistRoot = [System.IO.Path]::GetFullPath($distRoot)
$resolvedPackageDir = [System.IO.Path]::GetFullPath($packageDir)
$rootPrefix = $resolvedRoot.TrimEnd([System.IO.Path]::DirectorySeparatorChar) + [System.IO.Path]::DirectorySeparatorChar
if (($resolvedDistRoot -ne $resolvedRoot) -and (-not $resolvedDistRoot.StartsWith($rootPrefix))) {
  throw "OutputDir fora do projeto: $resolvedDistRoot"
}
if (-not $resolvedPackageDir.StartsWith($rootPrefix)) {
  throw "packageDir fora do projeto: $resolvedPackageDir"
}

function Get-PackageChecksums([string]$PackageDir) {
  $resolvedPackage = [System.IO.Path]::GetFullPath($PackageDir)
  $prefix = $resolvedPackage.TrimEnd([System.IO.Path]::DirectorySeparatorChar) + [System.IO.Path]::DirectorySeparatorChar
  $items = [ordered]@{}
  Get-ChildItem -LiteralPath $resolvedPackage -Recurse -File |
    Where-Object { $_.Name -ne "manifest.json" } |
    Sort-Object FullName |
    ForEach-Object {
      $full = [System.IO.Path]::GetFullPath($_.FullName)
      if (-not $full.StartsWith($prefix)) {
        throw "arquivo fora do pacote: $full"
      }
      $relative = $full.Substring($prefix.Length).Replace('\', '/')
      $items[$relative] = (Get-FileHash -Algorithm SHA256 -LiteralPath $full).Hash.ToLowerInvariant()
    }
  return $items
}

& (Join-Path $PSScriptRoot "build.ps1") -OutputDir $packageBuildRel -Version $Version -Commit $Commit | Write-Host

if (Test-Path $packageDir) {
  Remove-Item -LiteralPath $packageDir -Recurse -Force
}

New-Item -ItemType Directory -Force -Path $packageDir | Out-Null
New-Item -ItemType Directory -Force -Path (Join-Path $packageDir "data") | Out-Null
New-Item -ItemType Directory -Force -Path (Join-Path $packageDir "logs") | Out-Null
New-Item -ItemType Directory -Force -Path (Join-Path $packageDir "projects") | Out-Null
New-Item -ItemType Directory -Force -Path (Join-Path $packageDir "knowledge") | Out-Null

$builtBinary = Join-Path $packageBuildDir "omni-bot-go.exe"
$packageBinary = Join-Path $packageDir "omni-bot-go.exe"
if (Test-Path -LiteralPath $builtBinary) {
  Copy-Item -LiteralPath $builtBinary -Destination $packageDir -Force
}
if (-not (Test-Path -LiteralPath $packageBinary)) {
  & (Join-Path $PSScriptRoot "build.ps1") -OutputDir (Join-Path $OutputDir $packageName) -Version $Version -Commit $Commit | Write-Host
}
if (-not (Test-Path -LiteralPath $packageBinary)) {
  throw "binario nao foi gerado no pacote: $packageBinary"
}
Copy-Item -LiteralPath (Join-Path $root "web") -Destination $packageDir -Recurse
New-Item -ItemType Directory -Force -Path (Join-Path $packageDir "plugins") | Out-Null
Copy-Item -Path (Join-Path $root "plugins\*.json") -Destination (Join-Path $packageDir "plugins")
Copy-Item -LiteralPath (Join-Path $root "commands") -Destination $packageDir -Recurse
Copy-Item -LiteralPath (Join-Path $root "docs") -Destination $packageDir -Recurse
if (Test-Path -LiteralPath (Join-Path $root "CHANGELOG.md")) {
  Copy-Item -LiteralPath (Join-Path $root "CHANGELOG.md") -Destination $packageDir
}
Copy-Item -Path (Join-Path $root "knowledge\*.pack.json") -Destination (Join-Path $packageDir "knowledge")

$runScript = @'
param(
  [string]$Addr = "127.0.0.1:8080"
)

$ErrorActionPreference = "Stop"

$root = $PSScriptRoot
New-Item -ItemType Directory -Force -Path (Join-Path $root "data") | Out-Null
New-Item -ItemType Directory -Force -Path (Join-Path $root "logs") | Out-Null
New-Item -ItemType Directory -Force -Path (Join-Path $root "projects") | Out-Null

Push-Location $root
try {
  $argsList = @(
    "-addr", $Addr,
    "-db", (Join-Path $root "data\loja.db"),
    "-plugins", (Join-Path $root "plugins"),
    "-log", (Join-Path $root "logs\omni-bot-go.log")
  )
  & (Join-Path $root "omni-bot-go.exe") @argsList
} finally {
  Pop-Location
}
'@
Set-Content -LiteralPath (Join-Path $packageDir "run.ps1") -Value $runScript -Encoding ASCII

$manifest = [ordered]@{
  app = "omni-bot-go"
  version = $Version
  commit = $Commit
  created_at = (Get-Date).ToUniversalTime().ToString("yyyy-MM-ddTHH:mm:ssZ")
  entrypoint = "omni-bot-go.exe"
  runtime_db = "data/loja.db"
  runtime_log = "logs/omni-bot-go.log"
  external_dependencies = @()
  packaged_assets = @("web", "plugins", "commands", "docs", "CHANGELOG.md", "knowledge/*.pack.json", "run.ps1")
  checksums = Get-PackageChecksums $packageDir
}
$manifest | ConvertTo-Json | Set-Content -LiteralPath (Join-Path $packageDir "manifest.json") -Encoding ASCII

if (-not (Test-Path -LiteralPath $packageBinary)) {
  & (Join-Path $PSScriptRoot "build.ps1") -OutputDir (Join-Path $OutputDir $packageName) -Version $Version -Commit $Commit | Write-Host
}
if (-not (Test-Path -LiteralPath $packageBinary)) {
  throw "binario nao foi gerado no pacote: $packageBinary"
}

Write-Host "package ok: $packageDir"
