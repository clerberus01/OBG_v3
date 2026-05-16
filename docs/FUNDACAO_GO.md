# Fundacao Go / Pastas / Build

## Pastas

- `cmd/`: entrada HTTP, dashboard, gerente e executor.
- `database/`: SQLite com `modernc.org/sqlite`, sem CGO.
- `engine/`: wrapper do Symbolic Engine v2.
- `engine/symbolic/`: planner, regras e templates deterministicos.
- `knowledge/`: Knowledge Packs, packs JIT locais e regras evolutivas.
- `mcp/`: registro e chamadas de plugins externos.
- `web/`: dashboard embarcado por arquivo estatico.
- `commands/`: YAMLs operacionais.
- `plugins/`: manifestos MCP/plugins.
- `plugins/sandbox/`: sandbox controlado para execucao de comandos e servicos.
- `data/`: banco runtime local, ignorado pelo Git.
- `logs/`: logs de processo/runtime, ignorado pelo Git.
- `projects/`: artefatos selados gerados em runtime, ignorado pelo Git.
- `bin/`: binarios gerados por build, ignorado pelo Git.
- `.gocache/`: cache local de build/teste, ignorado pelo Git.

## Comandos

```powershell
.\scripts\test.ps1
.\scripts\audit.ps1
.\scripts\build.ps1 -Version "0.1.0" -Commit "local"
.\bin\omni-bot-go.exe -version
.\scripts\run.ps1 -Addr "127.0.0.1:8090"
.\scripts\package.ps1 -Version "0.1.0" -Commit "local"
.\scripts\supervise.ps1 -Executable ".\bin\omni-bot-go.exe"
```

## Regras de build

- `CGO_ENABLED=0` e obrigatorio.
- SQLite deve continuar em `modernc.org/sqlite`.
- O build oficial sai em `bin/omni-bot-go.exe`.
- Nao use `go build ./cmd` como build final, porque isso gera `cmd.exe` na raiz.
- O build padrao nao remove simbolos/debug info; isso aumenta o arquivo, mas reduz chance de heuristica de antivirus. Use `.\scripts\build.ps1 -Strip` apenas para pacote compacto.
- Banco, cache e artefatos runtime ficam fora do versionamento.
- O banco padrao do runtime e `data/loja.db`.
- O log padrao do runtime e `logs/omni-bot-go.log`.
- O log de runtime rotaciona automaticamente ao iniciar quando passa de 10 MB, mantendo 3 arquivos antigos.
- `/api/health` deve responder `status=ok` antes de uso assistido.
- `/api/factory_series` cria lotes seriais de contratos com dependencia entre itens.
- O pacote distribuivel sai em `dist/omni-bot-go-<versao>/`.
- `scripts/supervise.ps1` nao compila nada; ele apenas supervisiona um executavel ja gerado e reinicia em caso de falha.
- Validacao externa com `go build`/`go test` fica desativada por padrao para evitar criacao de executaveis temporarios que podem acionar antivirus. Use `.\scripts\run.ps1 -EnableGoValidation` apenas quando precisar desse crivo.

## Antivirus

O projeto pode gerar executaveis Go locais em `bin/`, `dist/`, `tmp/` e caches de build. Esses arquivos sao artefatos gerados e podem ser removidos sem afetar o codigo-fonte. Se o antivirus alertar, limpe `.gocache/`, `tmp/`, `bin/`, `dist/` e qualquer `cmd.exe` gerado na raiz; depois gere novo pacote apenas com `.\scripts\package.ps1`.

## V2 / Symbolic Engine

- O runtime de producao nao usa GGUF, tokenizer, tensores, transformer ou APIs externas.
- O motor simbolico aplica regras locais e templates auditaveis.
- Knowledge Packs ficam persistidos no SQLite em `knowledge_packs`.
- O endpoint de inspecao do motor e `/api/engine`.
- O modo Fabrica em Serie roda sem goroutines extras; usa a mesma fila de workers e dependencias entre tarefas.
- Regras candidatas do self-evolving ficam pendentes ate aprovacao manual no dashboard ou em `/api/knowledge_candidates`.
