param(
  [Parameter(Mandatory=$true)]
  [string]$ZipPath,
  [string]$Addr = "127.0.0.1:18082"
)

$ErrorActionPreference = "Stop"

$root = Resolve-Path (Join-Path $PSScriptRoot "..")
$smokeRoot = Join-Path $root "tmp\smoke-release"
$packageDir = Join-Path $smokeRoot "omni-bot-go-2.0.0"
$resolvedZip = [System.IO.Path]::GetFullPath((Resolve-Path $ZipPath).Path)

function Assert-InRoot([string]$Path, [string]$RootPath) {
  $resolvedPath = [System.IO.Path]::GetFullPath($Path)
  $resolvedRootPath = [System.IO.Path]::GetFullPath($RootPath)
  $prefix = $resolvedRootPath.TrimEnd([System.IO.Path]::DirectorySeparatorChar) + [System.IO.Path]::DirectorySeparatorChar
  if (($resolvedPath -ne $resolvedRootPath) -and (-not $resolvedPath.StartsWith($prefix))) {
    throw "path fora do diretorio permitido: $resolvedPath"
  }
  return $resolvedPath
}

function Stop-PackageProcess([string]$PackageDir) {
  $exe = [System.IO.Path]::GetFullPath((Join-Path $PackageDir "omni-bot-go.exe"))
  Get-CimInstance Win32_Process |
    Where-Object { $_.ExecutablePath -eq $exe } |
    ForEach-Object { Stop-Process -Id $_.ProcessId -Force }
}

Assert-InRoot $smokeRoot $root | Out-Null
if (Test-Path -LiteralPath $smokeRoot) {
  Stop-PackageProcess $packageDir
  Remove-Item -LiteralPath $smokeRoot -Recurse -Force
}
New-Item -ItemType Directory -Force -Path $smokeRoot | Out-Null
Expand-Archive -LiteralPath $resolvedZip -DestinationPath $packageDir -Force

$proc = $null
try {
  $exe = Join-Path $packageDir "omni-bot-go.exe"
  $argsList = @(
    "-addr", $Addr,
    "-db", (Join-Path $packageDir "data\loja.db"),
    "-plugins", (Join-Path $packageDir "plugins"),
    "-log", (Join-Path $packageDir "logs\omni-bot-go.log")
  )
  $proc = Start-Process -FilePath $exe -ArgumentList $argsList -WorkingDirectory $packageDir -WindowStyle Hidden -PassThru
  $health = $null
  for ($i = 0; $i -lt 40; $i++) {
    Start-Sleep -Milliseconds 500
    if ($proc.HasExited) {
      throw "processo encerrou antes do smoke test: $($proc.ExitCode)"
    }
    try {
      $health = Invoke-RestMethod -Uri "http://$Addr/api/health" -TimeoutSec 2
      break
    } catch {}
  }
  if ($null -eq $health -or $health.status -ne "ok") {
    throw "health nao ficou ok"
  }

  $payload = @{
    template = "Gerar documento tecnico para {{item}}"
    items = @("Modulo A", "Modulo B")
  } | ConvertTo-Json
  $factory = Invoke-RestMethod -Method POST -Uri "http://$Addr/api/factory_series" -ContentType "application/json" -Body $payload -TimeoutSec 30
  if ($factory.mode -ne "factory-series" -or [int]$factory.count -ne 2 -or -not $factory.batch_id) {
    throw "factory_series inesperado: $($factory | ConvertTo-Json -Compress)"
  }

  $snapshot = Invoke-RestMethod -Uri "http://$Addr/api/snapshot" -TimeoutSec 30
  $contracts = @($snapshot.contracts)
  $tasks = @($snapshot.tasks)
  $factorySeries = @($snapshot.factory_series)
  if ($contracts.Count -lt 2) {
    throw "contratos insuficientes no smoke: $($contracts.Count)"
  }
  if ($tasks.Count -lt 8) {
    throw "tarefas insuficientes no smoke: $($tasks.Count)"
  }
  $batch = $factorySeries | Where-Object { $_.batch_id -eq $factory.batch_id } | Select-Object -First 1
  if (-not $batch) {
    throw "lote factory_series ausente no snapshot"
  }
  $factoryTasks = @($tasks | Where-Object { $_.payload.factory.batch_id -eq $factory.batch_id })
  if ($factoryTasks.Count -lt 8) {
    throw "tarefas do lote insuficientes: $($factoryTasks.Count)"
  }
  $firstByIndex = $factoryTasks | Sort-Object id | Select-Object -First 1
  if ($firstByIndex.payload.source -ne "factory_series") {
    throw "source factory_series ausente na tarefa"
  }

  [pscustomobject]@{
    status = "ok"
    zip = $resolvedZip
    package = [System.IO.Path]::GetFullPath($packageDir)
    health = $health.status
    batch_id = $factory.batch_id
    factory_count = [int]$factory.count
    contracts = $contracts.Count
    tasks = $tasks.Count
    factory_tasks = $factoryTasks.Count
    snapshot_factory_batches = $factorySeries.Count
  } | ConvertTo-Json
}
finally {
  if ($proc -and -not $proc.HasExited) {
    Stop-Process -Id $proc.Id -Force
  }
}
