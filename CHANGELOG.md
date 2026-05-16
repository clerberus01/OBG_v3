# Changelog

## v2.0.0-internal

- Removeu dependencias produtivas de GGUF/modelos externos; runtime usa Symbolic Engine deterministico.
- Adicionou Knowledge Packs iniciais, incluindo `web-services` e `local-tools`.
- Implementou sandbox controlado para plugins em `plugins/sandbox/sandbox.go`.
- Registrou comandos locais, servicos locais e servicos web no SQLite/dashboard.
- Adicionou permissoes por contrato e tarefa para chamadas de plugins.
- Adicionou suporte controlado a `git`, `docker/podman`, `curl`, scripts aprovados e endpoints web/local-service.
- Implementou self-evolving basico com regras candidatas pendentes e aprovacao manual.
- Adicionou modo Fabrica em Serie com lotes, contratos por item, dependencias seriais e resumo no dashboard.
- Adicionou crivo tecnico `scripts/audit.ps1` com `gofmt`, `go vet`, testes, varredura local e benchmarks RAM/CPU.

## Pendencias deliberadas

- `govulncheck` e `gosec` sao executados apenas quando estiverem instalados na maquina.
- Operacoes mutaveis de `git`, `docker` e `podman` permanecem bloqueadas por padrao ate existir um contrato/permissao especifica para esse risco.
- Empacotamento final deve ser rodado somente quando o antivirus corporativo nao remover os binarios gerados.
