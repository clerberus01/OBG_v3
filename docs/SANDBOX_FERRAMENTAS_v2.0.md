# Sandbox e Ferramentas v2.0

Data: 2026-05-18

## Objetivo

Este documento define o contrato de producao do Sandbox/Ferramentas do OBG v2.0.

O sandbox permite execucao controlada de ferramentas locais e servicos HTTP sem transformar o OBG em um executor livre de comandos. Toda ferramenta precisa ser declarada em manifesto, validada, registrada, executada com escopo e auditada no SQLite/dashboard.

## Arquitetura

Componentes principais:

- `plugins/sandbox/sandbox.go`: valida politica, comandos, workdir, env, endpoints, headers e executa processos locais.
- `mcp/registry.go`: carrega manifestos `plugins/*.json`, valida transportes, registra ferramentas e chama plugins.
- `cmd/manager.go`: aplica permissao por contrato/tarefa, bloqueia read-only, registra historico e publica snapshot.
- `database/sqlite_manager.go`: persiste chamadas, catalogo de ferramentas e permissoes.
- `web/index.html`: exibe manifestos, catalogo, permissoes e historico.

Fluxo de execucao:

1. Manifesto e carregado de `plugins/*.json`.
2. Registro formal e sincronizado em `plugin_command_registry`.
3. Chamada entra por `Manager.CallPlugin`.
4. `task_id` e `contract_id` sao validados.
5. Permissoes armazenadas sao resolvidas.
6. Permissao inline e permissao armazenada sao combinadas de forma restritiva.
7. `mcp.Registry` chama processo local ou servico HTTP.
8. Resultado e gravado em `plugin_calls`.
9. Snapshot atualiza dashboard.

## Contrato Formal de Permissoes

Toda execucao de ferramenta passa pelo sandbox antes de chegar ao sistema operacional ou a um servico HTTP.

Precedencia operacional:

1. Politica do plugin: define o teto maximo permitido pelo manifesto.
2. Permissao de contrato: estreita o teto para um contrato especifico.
3. Permissao de tarefa: tem prioridade sobre a permissao de contrato quando existir para a tarefa/tool.
4. Permissao inline da chamada: sempre combina de forma restritiva com a permissao armazenada mais especifica.

Regra pratica: a permissao efetiva nunca deve ampliar a politica do plugin. Ela so pode reduzir comandos, scripts, variaveis de ambiente, output e workdir.

## Regras de Escopo

- Chamada com `task_id` valida a tarefa, herda `contract_id` dela e rejeita divergencia entre tarefa e contrato.
- Tarefa `read_only` nao executa plugin.
- Permissao `task` exige `task_id`.
- Permissao `contract` exige `contract_id`.
- `plugin_id` e `tool` vazios viram wildcard `*`.
- Permissao desabilitada nao participa da resolucao.

## Registro Formal de Ferramentas

Toda ferramenta declarada em `plugins/*.json` deve aparecer em `plugin_command_registry` e no dashboard.

Campos obrigatorios do registro:

- `kind`: `local-command`, `local-script`, `local-service` ou `web-service`.
- `transport`: transporte real do manifesto.
- `target`: comando/endpoint auditavel.
- `status`: `enabled`, `blocked`, `controlled-command` ou `approved-script`.
- `enabled`: estado operacional do manifesto.
- `sandbox`: politica declarada no manifesto.
- `updated_at`: ultima sincronizacao do registro.

Regras:

- Manifesto desabilitado registra `status=blocked`.
- Script local aprovado registra `kind=local-script` e `status=approved-script`.
- Comando local allowlisted registra `status=controlled-command`.
- Servico local registra `kind=local-service`.
- Servico web registra `kind=web-service`.

## Politica Avancada de Comandos

Comandos destrutivos ou shells gerais sao bloqueados mesmo quando aparecem em allowlist:

- `cmd`, `bash`, `sh`
- `rm`, `del`, `erase`, `rd`, `rmdir`
- `format`, `mkfs`, `mount`
- `shutdown`, `taskkill`, `takeown`
- `reg`, `robocopy`, `rsync`, `scp`, `ssh`, `wsl`

Comandos controlados:

- `go`: somente `go test` e `go build`.
- `gofmt`: permitido.
- `powershell`/`pwsh`: somente com `-File` apontando para script aprovado; `-Command` e `-EncodedCommand` bloqueados.
- `git`: somente leitura; comandos mutaveis como `checkout`, `commit`, `push`, `pull`, `reset` e `clean` bloqueados.
- `docker`/`podman`: somente inspecao/leitura; `run`, `build`, `compose up` e `compose down` bloqueados.
- `curl`: somente `GET` e `HEAD`; escrita, upload, dados, proxy, config, netrc e headers customizados bloqueados.

## Servicos Locais e Web

Transportes HTTP seguem regras separadas do executor local:

- `local-service` aceita apenas `http`/`https` para `localhost`, `127.0.0.0/8`, `::1` ou outro loopback reconhecido.
- `web-service` aceita apenas `https` e rejeita localhost/loopback.
- Toda chamada de servico usa `POST` com envelope JSON-RPC `tools/call`.
- Headers tecnicos `X-OBG-Plugin-ID`, `X-OBG-Tool`, `X-OBG-Contract-ID` e `X-OBG-Task-ID` sao enviados automaticamente.
- Headers declarados no manifesto sao validados contra byte nulo e quebra de linha.
- Timeout maximo do manifesto: 300 segundos; default operacional: 30 segundos.
- Output de resposta HTTP e erro HTTP passa por `max_output_bytes`.
- HTTP fora de 2xx retorna erro e ainda preserva output truncado no resultado.

## Exemplos de Manifesto

Comando local controlado:

```json
{
  "id": "go-tests",
  "name": "Go Tests",
  "transport": "stdio-json",
  "command": "go",
  "args": ["test", "./..."],
  "enabled": true,
  "timeout_seconds": 120,
  "sandbox": {
    "allow_commands": ["go test"],
    "max_output_bytes": 16000
  },
  "tools": [
    {"name": "test", "description": "Executa go test controlado"}
  ]
}
```

Script local aprovado:

```json
{
  "id": "audit-script",
  "name": "Audit Script",
  "transport": "stdio-json",
  "command": "powershell",
  "args": ["-NoProfile", "-File", "scripts/audit.ps1"],
  "enabled": true,
  "timeout_seconds": 300,
  "sandbox": {
    "approved_scripts": ["scripts/audit.ps1"],
    "max_output_bytes": 32000
  },
  "tools": [
    {"name": "audit", "description": "Executa auditoria local aprovada"}
  ]
}
```

Servico local:

```json
{
  "id": "local-tool",
  "name": "Local Tool",
  "transport": "local-service",
  "endpoint": "http://127.0.0.1:9000/tool",
  "enabled": true,
  "timeout_seconds": 30,
  "sandbox": {
    "max_output_bytes": 16000
  },
  "tools": [
    {"name": "call", "description": "Chama ferramenta HTTP local"}
  ]
}
```

Servico web:

```json
{
  "id": "web-tool",
  "name": "Web Tool",
  "transport": "web-service",
  "endpoint": "https://example.com/tool",
  "headers": {
    "X-Tool-Version": "2"
  },
  "enabled": true,
  "timeout_seconds": 30,
  "sandbox": {
    "max_output_bytes": 16000
  },
  "tools": [
    {"name": "call", "description": "Chama servico web aprovado"}
  ]
}
```

## Exemplos Bloqueados

Devem falhar mesmo quando alguem tentar colocar na allowlist:

```text
cmd /c qualquer-coisa
powershell -EncodedCommand ...
powershell -Command ...
git push origin main
git reset --hard
docker run alpine
docker compose up
curl -X POST https://example.com
curl -o arquivo.txt https://example.com
curl -H "Authorization: token" https://example.com
```

## Historico e Auditoria

Toda chamada que chega ao executor de plugin deve registrar auditoria em `plugin_calls`.

Campos auditados:

- `plugin_id` e `tool`
- `transport`
- `contract_id` e `task_id`
- `input` estruturado, incluindo permissoes efetivas da chamada
- `output` estruturado/truncado
- `ok`
- `duration`
- `sandboxed`
- `work_dir`
- `error`
- `created_at`

Regras:

- Sucesso registra `ok=true`.
- Falha de execucao ou HTTP fora de 2xx registra `ok=false`, `error` e output truncado quando houver.
- Chamadas recusadas antes da execucao por tarefa `read_only` nao chegam ao executor e sao bloqueio de integridade, nao historico de ferramenta.
- `/api/snapshot` expoe `plugin_calls`, `plugin_command_registry` e `plugin_permission_scopes` para dashboard.

## Operacao No Dashboard

Zona 3, aba `Plugins`, mostra:

- ultimo resultado;
- registro de comandos locais e web;
- permissoes por contrato/tarefa;
- manifestos carregados;
- historico de chamadas.

Sinais esperados:

- `ativo`/`inativo`: estado do manifesto.
- `enabled`, `blocked`, `controlled-command`, `approved-script`: estado do registro.
- `local-command`, `local-script`, `local-service`, `web-service`: classe operacional.
- `sandboxed`: chamada passou pelo sandbox.
- `erro`: falha registrada em `plugin_calls`.

## Evidencias de Regressao

- `TestPluginCallCannotUseReadOnlyTaskScope`
- `TestCombinePluginPermissionsIsRestrictive`
- `TestStoredPluginPermissionPrecedenceTaskContractInline`
- `TestPluginPermissionScopesResolveTaskBeforeContract`
- `TestCallPluginAppliesContractTaskPermissions`
- `TestCallPluginPermissionNarrowsCommand`
- `TestCommandRegistrationsClassifyLocalAndWebTargets`
- `TestPluginCommandRegistryPersistsLocalAndWebCommands`
- `TestBlocksDangerousCommandsEvenWhenAllowlisted`
- `TestAllowsOnlyApprovedLocalScripts`
- `TestControlledGitSupport`
- `TestMutableGitCommandsRemainBlockedForV200`
- `TestControlledDockerPodmanSupport`
- `TestMutableContainerCommandsRemainBlockedForV200`
- `TestControlledCurlSupport`
- `TestCallLocalServicePlugin`
- `TestCallServicePluginTruncatesOutputAndReportsHTTPError`
- `TestLocalServiceRequiresLoopback`
- `TestWebServiceRequiresHTTPS`
- `TestWebServiceRejectsLoopbackAndInvalidHeaders`
- `TestServiceEndpointValidation`
- `TestHeaderValidation`
- `TestPluginCallHistoryStoresStructuredMetadata`
- `TestPluginCallHistoryAuditsSuccessAndFailure`

## Crivo Tecnico

Ultima execucao registrada: 2026-05-18.

Comandos executados:

- `go test ./plugins/sandbox`
- `go test ./mcp`
- `go test ./cmd -run "TestPlugin|TestStoredPlugin|TestAPIRuntimeEndpoints"`
- `scripts/audit.ps1`

Resultado:

- `gofmt`: aprovado.
- `go vet`: aprovado.
- suite Go completa: aprovada.
- `govulncheck`: sem vulnerabilidades.
- `gosec`: 0 issues.
- benchmarks CPU/RAM: executados para `engine/symbolic`, `knowledge` e `plugins/sandbox`.

Benchmarks relevantes do sandbox:

- `BenchmarkCommandAllowedControlledGit`: ~1.1-1.3 us/op, 1504 B/op.
- `BenchmarkPrepareScopedRequest`: ~108-125 us/op, 87751 B/op.

## Limites Conhecidos

- O sandbox controla politica de execucao, mas nao cria isolamento de kernel/VM.
- Processos locais rodam no sistema operacional da maquina, dentro do workdir controlado e com env reduzido.
- Servicos web dependem da disponibilidade e confiabilidade externa do endpoint.
- `web-service` exige HTTPS, mas validacao de identidade do provedor continua responsabilidade da rede/TLS do sistema.
- Comandos mutaveis de `git`, `docker` e `podman` permanecem bloqueados na v2.0.0.
- Operacoes que o antivirus corporativo interceptar podem exigir liberacao manual no ambiente.

## Checklist de Fechamento

- Manifesto existe em `plugins/*.json`.
- `enabled` esta correto.
- `transport` e valido.
- `timeout_seconds` esta entre 0 e 300.
- `sandbox.max_output_bytes` esta entre 1 e 65536.
- Comando/endpoint passa na politica do sandbox.
- Permissao por contrato/tarefa foi registrada quando necessaria.
- Chamada aparece em `plugin_calls`.
- Registro aparece em `plugin_command_registry`.
- Dashboard exibe historico e resultado.
