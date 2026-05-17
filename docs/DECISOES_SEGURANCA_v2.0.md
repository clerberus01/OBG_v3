# Decisoes de Seguranca v2.0

Data: 2026-05-16

## DSR-001: Operacoes Mutaveis de Git e Containers

Status: aprovado para v2.0.0.

Decisao: a release v2.0.0 bloqueia por padrao comandos mutaveis de `git`, `docker` e `podman`, mesmo quando alguem tenta inclui-los em `allow_commands`.

Permitido na v2.0.0:

- `git status`, `git diff`, `git log`, `git show`, `git branch`, `git rev-parse`, `git ls-files`, `git remote -v`, `git version`.
- `docker/podman ps`, `images`, `inspect`, `logs`, `info`, `version`.
- `docker/podman compose ps`, `compose logs`, `compose config`.

Bloqueado na v2.0.0:

- `git commit`, `git push`, `git pull`, `git checkout`, `git reset`, `git clean` e comandos equivalentes que alteram historico, working tree ou remoto.
- `docker/podman run`, `build`, `push`, `pull`, `exec`, `start`, `stop`, `rm`, `rmi`.
- `docker/podman compose up`, `down`, `build`, `pull`, `push`, `restart`.

Justificativa:

- O Contrato Universal ainda nao possui niveis formais de risco como `inspect`, `build`, `mutate` e `network`.
- Operacoes mutaveis podem alterar repositorios, containers, imagens, volumes, rede local ou ambiente corporativo.
- A v2.0.0 prioriza determinismo, auditoria e reproducibilidade.

Condicao para reabrir:

- Criar politica versionada de permissoes por nivel de risco.
- Persistir comandos bloqueados e liberados no SQLite.
- Expor autorizacoes e bloqueios no dashboard.
- Adicionar testes negativos e positivos por nivel.
- Exigir aprovacao explicita por contrato/tarefa.

Testes de regressao:

- `TestMutableGitCommandsRemainBlockedForV200`
- `TestMutableContainerCommandsRemainBlockedForV200`
