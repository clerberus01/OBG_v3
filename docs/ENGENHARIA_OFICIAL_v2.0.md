# Engenharia Oficial v2.0

Data: 2026-05-15

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

## Regras de Producao

- Se nao esta no SQLite, nao aconteceu.
- Contratos sao selados por hash imutavel.
- Tarefas aprovadas ficam read-only.
- Handoffs trafegam como relatorios de estado JSON no SQLite.
- O watchdog bloqueia tarefas apos 3 falhas.
- O motor simbolico aplica apenas regras locais e auditaveis.
- Plugins executam apenas com sandbox, allowlist e permissoes de contrato/tarefa.
- Operacoes mutaveis de `git`, `docker` e `podman` ficam bloqueadas por padrao; liberacao exige decisao explicita de contrato e novo crivo.

## Continuidade v2

- Fase 0 concluida: decoupling total de GGUF, Symbolic Engine, SQLite e dashboard 3 zonas.
- Fase 1 avancada: sandbox, registro de comandos, historico no SQLite e permissoes por escopo.
- Fase 2 avancada: packs por dominio, JIT persistente, self-evolving basico e crivo tecnico.
- Fase 3 em andamento: Fabrica em Serie pronta em base serial; exportacao/release final ainda depende de ambiente sem bloqueio de antivirus.

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
