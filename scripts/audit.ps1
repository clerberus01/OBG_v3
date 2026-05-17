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
$goRoot = (& $go env GOROOT).Trim()
$goBin = Join-Path $goRoot "bin"
if ($goBin -and (Test-Path -LiteralPath $goBin)) {
  $env:PATH = "$goBin;$env:PATH"
}

function Invoke-Checked {
  param(
    [Parameter(Mandatory=$true)]
    [scriptblock]$Command,
    [Parameter(Mandatory=$true)]
    [string]$Name
  )
  & $Command
  if ($LASTEXITCODE -ne 0) {
    throw "$Name falhou com codigo $LASTEXITCODE"
  }
}

Push-Location $root
try {
  $packages = @(& $go list ./... | Where-Object { $_ -notmatch '/OBG' })
  if ($LASTEXITCODE -ne 0 -or $packages.Count -eq 0) {
    throw "go list nao encontrou pacotes do projeto principal"
  }
  $securityDirs = @("./cmd", "./database", "./engine", "./engine/symbolic", "./knowledge", "./mcp", "./plugins/sandbox")

  Write-Host "== lint: gofmt =="
  $gofmt = Join-Path $goBin "gofmt.exe"
  if (-not (Test-Path -LiteralPath $gofmt)) { $gofmt = "gofmt" }
  $unformatted = & $gofmt -l (Get-ChildItem -Recurse -Filter *.go -File | ForEach-Object { $_.FullName })
  if ($unformatted) {
    $unformatted | ForEach-Object { Write-Host $_ }
    throw "gofmt pendente"
  }

  Write-Host "== lint: go vet =="
  Invoke-Checked { & $go vet @packages } "go vet"

  Write-Host "== testes =="
  Invoke-Checked { & $go test @packages } "go test"

  Write-Host "== seguranca: varredura local =="
  $dangerPatterns = @(
    "Invoke-Expression",
    "\biex\b",
    "rm -rf",
    "Remove-Item\s+.*-Recurse\s+.*-Force",
    "curl\s+.*\|\s*(sh|bash|powershell|pwsh)",
    "os\.RemoveAll",
    "syscall\.",
    "unsafe\."
  )
  foreach ($pattern in $dangerPatterns) {
    $matches = rg --glob '!OBG*/**' --glob '!dist/**' --glob '!tmp/**' --glob '!scripts/audit.ps1' --glob '!*.sum' -n $pattern .
    if ($LASTEXITCODE -eq 0) {
      $unexpected = @($matches | Where-Object {
        $_ -notmatch '^\.\\scripts\\clean\.ps1:\d+:\s+Remove-Item -LiteralPath \$resolvedPath -Recurse -Force$' -and
        $_ -notmatch '^\.\\scripts\\package\.ps1:\d+:\s+Remove-Item -LiteralPath \$packageDir -Recurse -Force$' -and
        $_ -notmatch '^\.\\scripts\\verify-release\.ps1:\d+:\s+Remove-Item -LiteralPath \$verifyRoot -Recurse -Force$' -and
        $_ -notmatch '^\.\\scripts\\smoke-release\.ps1:\d+:\s+Remove-Item -LiteralPath \$smokeRoot -Recurse -Force$'
      })
      if ($unexpected.Count -gt 0) {
        $unexpected | ForEach-Object { Write-Host $_ }
        throw "padrao perigoso encontrado: $pattern"
      }
    }
    if ($LASTEXITCODE -gt 1) {
      throw "falha ao executar rg para $pattern"
    }
  }

  $govulncheck = Get-Command govulncheck -ErrorAction SilentlyContinue
  if ($govulncheck) {
    Write-Host "== seguranca: govulncheck =="
    Invoke-Checked { govulncheck @packages } "govulncheck"
  } else {
    Write-Host "== seguranca: govulncheck indisponivel; etapa opcional ignorada =="
  }

  $gosec = Get-Command gosec -ErrorAction SilentlyContinue
  if ($gosec) {
    Write-Host "== seguranca: gosec =="
    Invoke-Checked { gosec @securityDirs } "gosec"
  } else {
    Write-Host "== seguranca: gosec indisponivel; etapa opcional ignorada =="
  }

  Write-Host "== benchmarks: CPU/RAM =="
  Invoke-Checked { & $go test ./engine/symbolic ./knowledge ./plugins/sandbox -run '^$' -bench . -benchmem -count 3 } "benchmarks"
}
finally {
  Pop-Location
}
