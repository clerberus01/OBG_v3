# Orçamento de Benchmarks v2.0

Data: 2026-05-18

## Objetivo

Formalizar limites de CPU/RAM para o crivo técnico da release interna `v2.0.0`.

O arquivo fonte do orçamento é `scripts/benchmark-budget.json`. O validador é `scripts/check-benchmark-budget.ps1` e roda automaticamente dentro de `scripts/audit.ps1`.

## Critério de Reprovação

O crivo reprova quando qualquer benchmark obrigatório:

- não aparece na saída de `go test -bench`;
- ultrapassa `max_ns_per_op`;
- ultrapassa `max_bytes_per_op`;
- ultrapassa `max_allocs_per_op`.

## Limites Ativos

| Pacote | Benchmark | CPU máximo | RAM máxima | Alocações máximas |
| --- | --- | ---: | ---: | ---: |
| `engine/symbolic` | `BenchmarkPlanHierarchical` | 500000 ns/op | 300000 B/op | 2500 allocs/op |
| `engine/symbolic` | `BenchmarkExecuteTask` | 120000 ns/op | 80000 B/op | 600 allocs/op |
| `knowledge` | `BenchmarkLoadPackFromFile` | 3000000 ns/op | 10000 B/op | 80 allocs/op |
| `knowledge` | `BenchmarkSearchWithOptions` | 250000 ns/op | 60000 B/op | 1000 allocs/op |
| `plugins/sandbox` | `BenchmarkCommandAllowedControlledGit` | 10000 ns/op | 5000 B/op | 30 allocs/op |
| `plugins/sandbox` | `BenchmarkPrepareScopedRequest` | 500000 ns/op | 150000 B/op | 1200 allocs/op |

## Última Evidência

Em 2026-05-18, `scripts/audit.ps1` aprovou:

- `gofmt`;
- `go vet`;
- `go test`;
- varredura local de segurança;
- `govulncheck`;
- `gosec`;
- benchmarks com `-benchmem -count 3`;
- orçamento CPU/RAM com 6 limites verificados.
