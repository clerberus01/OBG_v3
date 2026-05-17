# Checklist Pos-Dashboard v2.0.0

Data: 2026-05-17

## Fechado em 100%

- Runtime de producao sem GGUF, tokenizer, tensores, transformer ou modelo externo.
- Symbolic Engine como motor produtivo unico.
- Contrato Universal persistido no SQLite com hash imutavel.
- Tarefas aprovadas em modo read-only.
- Watchdog com contador por tarefa, motivo de falha e bloqueio apos 3 tentativas.
- Knowledge Packs carregados do disco, persistidos no SQLite e buscaveis.
- Regras candidatas do self-evolving ficam pendentes ate aprovacao manual.
- Sandbox controlado para plugins com allowlist, denylist e permissoes por contrato/tarefa.
- Comandos locais, servicos locais e servicos web registrados formalmente.
- Fábrica em Serie com contratos por item e dependencias seriais sem criar goroutines extras.
- `/api/health`, `/api/engine` e `/api/snapshot` validados.
- `/api/snapshot` expõe `contract_summaries`, `task_dependencies` e `watchdog_events`.
- Dashboard v2 completo nas 3 zonas:
  - Balcao: interrogatorio, previa revisavel e confirmacao antes de selar.
  - Quadro de Obras: tarefas por contrato, dominios, papeis, dependencias e watchdog.
  - Mostruario & Auditoria: artefatos, handoffs, Knowledge Packs, metricas, logs e filtros.
- Pacote final `dist/omni-bot-go-2.0.0.zip` gerado com manifesto e checksums.
- `scripts/verify-release.ps1` aprovado.
- `scripts/smoke-release.ps1` aprovado a partir do ZIP final.

## Melhorias Futuras, Nao Bloqueadoras

- Teste visual automatizado com Playwright em ambiente com navegador headless funcional.
- Exportacao do dashboard para relatorio HTML/PDF estatico de auditoria.
- Filtros persistidos no navegador para a Zona 3.
- Ordenacao configuravel em tabelas/listas de artefatos, handoffs e logs.
- Politica versionada mais granular para comandos mutaveis de `git`, `docker` e `podman`.
- Niveis formais de permissao como `inspect`, `build`, `mutate` e `network`.
- Regras de dashboard para comparar lotes da Fabrica em Serie.
- Paginação ou virtualizacao se o SQLite acumular milhares de tarefas/logs.

## Limitacoes Operacionais Conhecidas

- O antivirus corporativo pode pedir liberacao temporaria ao gerar ou executar `omni-bot-go.exe`.
- Chrome/Edge headless falharam neste ambiente por erro de GPU; `npx playwright` nao respondeu dentro do tempo.
- `govulncheck` e `gosec` dependem de estarem instalados para o crivo completo.
- Operacoes mutaveis de `git`, `docker` e `podman` permanecem bloqueadas por decisao de seguranca da v2.0.0.
- A release e local-first; integracoes web externas dependem de permissao explicita do sandbox e do ambiente de rede.

## Conclusao

A v2.0.0 esta pronta para validacao interna. Os itens restantes sao evolucoes controladas ou limitacoes do ambiente corporativo, nao bloqueadores da release atual.
