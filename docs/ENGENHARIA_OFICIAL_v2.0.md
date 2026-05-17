# Engenharia Oficial v2.0

Data: 2026-05-16

## Decisao de Arquitetura

O Omni-Bot Go v2 opera como binario unico em Go puro, local e deterministico. A Fase 0 remove o acoplamento de producao com GGUF, tokenizer, tensores, transformer e qualquer modelo neural externo.

## Componentes

- `cmd/`: dashboard, API local, gerente, swarm e crivo tecnico.
- `engine/`: wrapper do motor simbolico.
- `engine/symbolic/`: planner, regras, templates e execucao deterministica.
- `database/`: SQLite puro com `modernc.org/sqlite`.
- `knowledge/`: Knowledge Packs e regras JIT com expiracao.
- `plugins/sandbox/`: sandbox controlado para comandos locais, scripts aprovados e servicos.
- `mcp/`: registro formal de comandos locais, local-service e web-service.
- `web/`: dashboard local em 3 zonas.

## Dashboard v2

O dashboard v2 esta dividido em 3 zonas operacionais:

- Balcao: entrada de contrato, interrogatorio guiado, previa revisavel e confirmacao antes de selar.
- Quadro de Obras: tarefas por contrato, papeis, dominios, dependencias visuais, progresso e watchdog.
- Mostruario & Auditoria: artefatos, handoffs, plugins, Knowledge Packs, metricas, logs e filtros locais.

O dashboard consome `GET /api/snapshot` como contrato principal de leitura. O snapshot expõe `contract_summaries`, `task_dependencies` e `watchdog_events` para evitar reconstrução frágil de estado no frontend.

## Regras de Producao

- Se nao esta no SQLite, nao aconteceu.
- Contratos sao selados por hash imutavel.
- Tarefas aprovadas ficam read-only.
- Handoffs trafegam como relatorios de estado JSON no SQLite.
- O watchdog bloqueia tarefas apos 3 falhas.
- O motor simbolico aplica apenas regras locais e auditaveis.
- Plugins executam apenas com sandbox, allowlist e permissoes de contrato/tarefa.
- Operacoes mutaveis de `git`, `docker` e `podman` ficam bloqueadas por padrao; liberacao exige decisao explicita de contrato e novo crivo.

Decisao formal: ver `docs/DECISOES_SEGURANCA_v2.0.md`.

## Continuidade v2

- Fase 0 concluida: decoupling total de GGUF, Symbolic Engine, SQLite e dashboard 3 zonas.
- Fase 1 avancada: sandbox, registro de comandos, historico no SQLite e permissoes por escopo.
- Fase 2 avancada: packs por dominio, JIT persistente, self-evolving basico e crivo tecnico.
- Fase 3 concluida para v2.0.0: Fabrica em Serie pronta em base serial, pacote final gerado, verificado e aprovado em smoke funcional.

## Fabrica em Serie

O modo Fabrica em Serie cria um lote deterministico a partir de um template e uma lista de itens. Cada item vira um contrato selado proprio. A primeira tarefa do contrato seguinte depende da ultima tarefa do contrato anterior, preservando execucao serial sem criar goroutines extras.

Endpoint: `POST /api/factory_series`.

Payload:

```json
{
  "template": "Gerar documento tecnico para {{item}}",
  "items": ["Modulo A", "Modulo B"],
  "constraints": "Opcional",
  "deliverables": "Opcional"
}
```

Placeholders: `{{item}}`, `{{index}}`, `{{total}}`, `{{batch}}`.

## Crivo Tecnico

Use `scripts/audit.ps1` para lint, seguranca local e benchmarks RAM/CPU:

```powershell
.\scripts\audit.ps1
```

O script executa `govulncheck` e `gosec` somente quando essas ferramentas existem no ambiente.

## Release Interna v2.0.0

Artefato final:

- `dist/omni-bot-go-2.0.0/`
- `dist/omni-bot-go-2.0.0.zip`

Validacoes executadas:

- `scripts/test.ps1`: suite Go completa.
- `scripts/audit.ps1`: gofmt, go vet, testes, `govulncheck`, `gosec` e benchmark basico quando ferramentas existem.
- `scripts/verify-release.ps1`: extracao limpa, checksums do manifesto, health, engine simbolico, packs e dashboard.
- `scripts/smoke-release.ps1`: fluxo funcional com Fabrica em Serie criando contratos e tarefas a partir do ZIP.
- Verificacao do dashboard: sintaxe JavaScript, HTML servido pelo binario atual e snapshot populado com contratos, tarefas, resumos e dependencias.

Observacao operacional: em ambiente corporativo, o antivirus pode solicitar liberacao temporaria ao executar ou gerar o binario. Isso nao altera o conteudo da release; apenas exige autorizacao local para a execucao do `.exe`.
