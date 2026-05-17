param(
  [Parameter(Mandatory=$true)]
  [string]$ZipPath,
  [string]$Addr = "127.0.0.1:18081"
)

$ErrorActionPreference = "Stop"

$root = Resolve-Path (Join-Path $PSScriptRoot "..")
$resolvedRoot = [System.IO.Path]::GetFullPath($root)
$resolvedZip = [System.IO.Path]::GetFullPath((Resolve-Path $ZipPath).Path)
$verifyRoot = Join-Path $root "tmp\verify-release"
$extractDir = Join-Path $verifyRoot "omni-bot-go-2.0.0"

function Assert-InRoot([string]$Path, [string]$RootPath) {
  $resolvedPath = [System.IO.Path]::GetFullPath($Path)
  $resolvedRootPath = [System.IO.Path]::GetFullPath($RootPath)
  $prefix = $resolvedRootPath.TrimEnd([System.IO.Path]::DirectorySeparatorChar) + [System.IO.Path]::DirectorySeparatorChar
  if (($resolvedPath -ne $resolvedRootPath) -and (-not $resolvedPath.StartsWith($prefix))) {
    throw "path fora do diretorio permitido: $resolvedPath"
  }
  return $resolvedPath
}

function Stop-ExtractedProcess([string]$PackageDir) {
  $exe = [System.IO.Path]::GetFullPath((Join-Path $PackageDir "omni-bot-go.exe"))
  Get-CimInstance Win32_Process |
    Where-Object { $_.ExecutablePath -eq $exe } |
    ForEach-Object { Stop-Process -Id $_.ProcessId -Force }
}

Assert-InRoot $verifyRoot $root | Out-Null
if (Test-Path -LiteralPath $verifyRoot) {
  Stop-ExtractedProcess $extractDir
  Remove-Item -LiteralPath $verifyRoot -Recurse -Force
}
New-Item -ItemType Directory -Force -Path $verifyRoot | Out-Null
Expand-Archive -LiteralPath $resolvedZip -DestinationPath $extractDir -Force

$manifestPath = Join-Path $extractDir "manifest.json"
if (-not (Test-Path -LiteralPath $manifestPath)) {
  throw "manifest.json ausente no ZIP"
}
$manifest = Get-Content -Raw -LiteralPath $manifestPath | ConvertFrom-Json
if ($manifest.app -ne "omni-bot-go") {
  throw "manifest app inesperado: $($manifest.app)"
}
if (-not $manifest.checksums) {
  throw "manifest sem checksums"
}

$packageRoot = [System.IO.Path]::GetFullPath($extractDir)
foreach ($item in $manifest.checksums.PSObject.Properties) {
  $relative = $item.Name
  if ($relative.Contains("..") -or [System.IO.Path]::IsPathRooted($relative)) {
    throw "checksum com path invalido: $relative"
  }
  $path = Assert-InRoot (Join-Path $packageRoot ($relative.Replace('/', '\'))) $packageRoot
  if (-not (Test-Path -LiteralPath $path)) {
    throw "arquivo do checksum ausente: $relative"
  }
  $actual = (Get-FileHash -Algorithm SHA256 -LiteralPath $path).Hash.ToLowerInvariant()
  if ($actual -ne $item.Value) {
    throw "checksum divergente: $relative"
  }
}

$required = @("omni-bot-go.exe", "run.ps1", "web\index.html", "plugins\example.echo.json", "commands\example.yaml", "docs\FUNDACAO_GO.md", "CHANGELOG.md")
foreach ($relative in $required) {
  if (-not (Test-Path -LiteralPath (Join-Path $packageRoot $relative))) {
    throw "arquivo obrigatorio ausente: $relative"
  }
}

$proc = $null
try {
  $exe = Join-Path $packageRoot "omni-bot-go.exe"
  $argsList = @(
    "-addr", $Addr,
    "-db", (Join-Path $packageRoot "data\loja.db"),
    "-plugins", (Join-Path $packageRoot "plugins"),
    "-log", (Join-Path $packageRoot "logs\omni-bot-go.log")
  )
  $proc = Start-Process -FilePath $exe -ArgumentList $argsList -WorkingDirectory $packageRoot -WindowStyle Hidden -PassThru
  $health = $null
  for ($i = 0; $i -lt 40; $i++) {
    Start-Sleep -Milliseconds 500
    if ($proc.HasExited) {
      throw "processo encerrou antes do health check: $($proc.ExitCode)"
    }
    try {
      $health = Invoke-RestMethod -Uri "http://$Addr/api/health" -TimeoutSec 2
      break
    } catch {}
  }
  if ($null -eq $health) {
    throw "health nao respondeu"
  }
  $engine = Invoke-RestMethod -Uri "http://$Addr/api/engine" -TimeoutSec 2
  $snapshot = Invoke-RestMethod -Uri "http://$Addr/api/snapshot" -TimeoutSec 2
  $dashboard = (New-Object System.Net.WebClient).DownloadString("http://$Addr/")

  if ($health.status -ne "ok") {
    throw "health inesperado: $($health.status)"
  }
  if ($engine.mode -ne "symbolic") {
    throw "engine mode inesperado: $($engine.mode)"
  }
  if ([int]$engine.pack_count -lt 7) {
    throw "packs iniciais nao carregados: $($engine.pack_count)"
  }
  if ($snapshot.PSObject.Properties.Name -contains "model") {
    throw "snapshot nao deve expor chave model"
  }
  if (-not $dashboard.Contains("Omni-Bot Go")) {
    throw "dashboard HTML inesperado"
  }

  [pscustomobject]@{
    status = "ok"
    zip = $resolvedZip
    extracted_to = $packageRoot
    checksums = @($manifest.checksums.PSObject.Properties).Count
    health = $health.status
    engine_mode = $engine.mode
    pack_count = [int]$engine.pack_count
    snapshot_has_model = $false
    dashboard_loaded = $true
  } | ConvertTo-Json
}
finally {
  if ($proc -and -not $proc.HasExited) {
    Stop-Process -Id $proc.Id -Force
  }
}
