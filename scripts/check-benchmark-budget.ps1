param(
  [Parameter(Mandatory=$true)]
  [string]$InputPath,
  [Parameter(Mandatory=$true)]
  [string]$BudgetPath
)

$ErrorActionPreference = "Stop"

if (-not (Test-Path -LiteralPath $InputPath)) {
  throw "arquivo de benchmark nao encontrado: $InputPath"
}
if (-not (Test-Path -LiteralPath $BudgetPath)) {
  throw "orcamento de benchmark nao encontrado: $BudgetPath"
}

$budget = Get-Content -LiteralPath $BudgetPath -Raw | ConvertFrom-Json
$lines = Get-Content -LiteralPath $InputPath
$currentPackage = ""
$results = @{}

foreach ($line in $lines) {
  if ($line -match '^pkg:\s+(.+)$') {
    $currentPackage = $Matches[1].Trim()
    continue
  }
  if ($line -match '^(Benchmark\S+)-\d+\s+\d+\s+([0-9.]+)\s+ns/op\s+([0-9.]+)\s+B/op\s+([0-9.]+)\s+allocs/op') {
    $name = $Matches[1]
    $key = "$currentPackage/$name"
    $results[$key] = [pscustomobject]@{
      Package = $currentPackage
      Name = $name
      NsPerOp = [double]$Matches[2]
      BytesPerOp = [double]$Matches[3]
      AllocsPerOp = [double]$Matches[4]
    }
  }
}

$failures = @()
foreach ($item in $budget.benchmarks) {
  $key = "$($item.package)/$($item.name)"
  if (-not $results.ContainsKey($key)) {
    $failures += "benchmark ausente: $key"
    continue
  }
  $actual = $results[$key]
  if ($actual.NsPerOp -gt [double]$item.max_ns_per_op) {
    $failures += "$key CPU $($actual.NsPerOp) ns/op > $($item.max_ns_per_op)"
  }
  if ($actual.BytesPerOp -gt [double]$item.max_bytes_per_op) {
    $failures += "$key RAM $($actual.BytesPerOp) B/op > $($item.max_bytes_per_op)"
  }
  if ($actual.AllocsPerOp -gt [double]$item.max_allocs_per_op) {
    $failures += "$key allocs $($actual.AllocsPerOp) allocs/op > $($item.max_allocs_per_op)"
  }
}

if ($failures.Count -gt 0) {
  $failures | ForEach-Object { Write-Host $_ }
  throw "orcamento de benchmark reprovado"
}

Write-Host "orcamento de benchmark aprovado: $($budget.benchmarks.Count) limites verificados"
