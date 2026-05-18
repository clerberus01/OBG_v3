# Changelog

## v2.0.0-internal

- Removeu dependencias produtivas de GGUF/modelos externos; runtime usa Symbolic Engine deterministico.
- Adicionou Knowledge Packs iniciais, incluindo `web-services` e `local-tools`.
- Adicionou Knowledge Packs dedicados para `analysis`, `strategy` e `design`.
- Adicionou testes especificos de carregamento, persistencia, busca e aplicacao dos packs `analysis`, `strategy` e `design`.
- Implementou sandbox controlado para plugins em `plugins/sandbox/sandbox.go`.
- Registrou comandos locais, servicos locais e servicos web no SQLite/dashboard.
- Adicionou permissoes por contrato e tarefa para chamadas de plugins.
- Adicionou suporte controlado a `git`, `docker/podman`, `curl`, scripts aprovados e endpoints web/local-service.
- Implementou self-evolving basico com regras candidatas pendentes e aprovacao manual.
- Adicionou modo Fabrica em Serie com lotes, contratos por item, dependencias seriais e resumo no dashboard.
- Adicionou crivo tecnico `scripts/audit.ps1` com `gofmt`, `go vet`, testes, varredura local e benchmarks RAM/CPU.
- Adicionou orcamento formal de benchmarks CPU/RAM com reprovacao automatica no crivo tecnico.
- Formalizou a decisao de seguranca v2.0 que mantem `git`, `docker` e `podman` mutaveis bloqueados por padrao.
- Atualizou o crivo para Go 1.26.3+, `govulncheck` e `gosec` com 0 achados no projeto principal.
- Gerou o pacote final `dist/omni-bot-go-2.0.0.zip` com manifesto, checksums e conteudo runtime limpo.
- Adicionou smoke funcional de release em `scripts/smoke-release.ps1`, validando health, engine simbolico, dashboard e Fabrica em Serie a partir do ZIP.
- Corrigiu concorrencia entre criacao de contratos/tarefas e scheduler para evitar `SQLITE_BUSY` durante lotes da Fabrica em Serie.
- Completou o dashboard v2 nas 3 zonas: Balcao, Quadro de Obras, Mostruario & Auditoria.
- Expandiu `/api/snapshot` com resumos de contrato, dependencias normalizadas e eventos persistidos do watchdog.
- Melhorou responsividade do dashboard e adicionou filtros locais para auditoria, logs, handoffs, artefatos e regras candidatas.
- Fechou a release interna v2.0.0 em 2026-05-17 com auditoria final aprovada, ZIP final com 19 arquivos e manifesto com 18 checksums.

## Pendencias deliberadas

- `govulncheck` e `gosec` sao executados apenas quando estiverem instalados na maquina.
- Operacoes mutaveis de `git`, `docker` e `podman` permanecem bloqueadas por padrao ate existir um contrato/permissao especifica para esse risco.
- Em ambiente corporativo, o antivirus pode pedir liberacao temporaria ao gerar/executar o binario do pacote.
- Chrome/Edge headless podem falhar por GPU neste ambiente corporativo; a validacao visual automatica depende de ambiente com navegador headless funcional.
