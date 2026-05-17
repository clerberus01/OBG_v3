# Status da Release v2.0.0

Data: 2026-05-17

## Resultado

Status: release interna pronta.

Artefatos:

- `dist/omni-bot-go-2.0.0/`
- `dist/omni-bot-go-2.0.0.zip`

## Evidencias

- Suite Go completa aprovada por `scripts/test.ps1`.
- Auditoria local aprovada por `scripts/audit.ps1` com `govulncheck` e `gosec` disponiveis no ambiente.
- Pacote verificado por `scripts/verify-release.ps1` em 2026-05-17.
- Smoke funcional aprovado por `scripts/smoke-release.ps1` em 2026-05-17.
- Auditoria final aprovada por `scripts/audit.ps1` em 2026-05-17.
- ZIP final reempacotado em 2026-05-17 com 19 arquivos e manifesto com 18 checksums.
- Dashboard v2 revisado nas 3 zonas e incluído no ZIP final.

Ultimo smoke funcional:

```json
{
  "status": "ok",
  "zip": "D:\\omni-bot-go\\dist\\omni-bot-go-2.0.0.zip",
  "factory_count": 2,
  "contracts": 2,
  "tasks": 20,
  "factory_tasks": 20,
  "snapshot_factory_batches": 1
}
```

Ultima verificacao de release:

```json
{
  "status": "ok",
  "checksums": 18,
  "health": "ok",
  "engine_mode": "symbolic",
  "pack_count": 7,
  "snapshot_has_model": false,
  "dashboard_loaded": true
}
```

## Escopo Entregue

- Runtime simbolico sem dependencia produtiva de GGUF/modelos.
- Contratos imutaveis com hash no SQLite.
- Tarefas read-only apos aprovacao.
- Watchdog com bloqueio apos 3 falhas.
- Knowledge Packs iniciais e persistencia no SQLite.
- Self-evolving basico com regras candidatas pendentes.
- Sandbox controlado com allowlist, denylist e permissoes por contrato/tarefa.
- Registro formal de comandos locais, servicos locais e servicos web.
- Dashboard em 3 zonas: Balcao, Quadro de Obras, Mostruario & Auditoria.
- Balcao com progresso de interrogatorio, revisao formal e confirmacao antes de selar contrato.
- Quadro de Obras com tarefas por contrato, resumo operacional, dependencias visuais e watchdog persistido.
- Mostruario & Auditoria com filtros para artefatos, handoffs, Knowledge Packs, metricas e logs.
- Fabrica em Serie com contratos por item e dependencias seriais.
- Scripts de build, teste, auditoria, pacote, verificacao e smoke.

## Observacoes Operacionais

- Em ambiente corporativo, o antivirus pode exigir liberacao temporaria ao gerar ou executar `omni-bot-go.exe`.
- A verificacao automatica por screenshot ficou limitada neste ambiente porque Chrome/Edge headless falharam por GPU e `npx playwright` nao respondeu dentro do tempo.
- Operacoes mutaveis de `git`, `docker` e `podman` continuam bloqueadas por padrao na v2.0.0.
- `govulncheck` e `gosec` sao executados pelo crivo quando estiverem instalados na maquina.

Checklist honesto pos-dashboard: `docs/CHECKLIST_POS_DASHBOARD_v2.0.0.md`.
