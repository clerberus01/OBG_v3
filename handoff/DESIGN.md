# Sistema de Handoffs Melhorado

## Visão Geral

Sistema estruturado de transferência de dados entre tarefas com rastreabilidade completa, integridade por hash e vínculo origem/destino explícito.

---

## 1. Envelope JSON Padronizado

Todo handoff segue este envelope:

```json
{
  "metadata": {
    "id": "hf_123",
    "version": "1.0",
    "contract_id": 42,
    "source": {
      "task_id": 10,
      "role": "gerente",
      "timestamp": "2026-05-15T10:30:00Z"
    },
    "destination": {
      "task_id": 11,
      "role": "arquiteto",
      "expected": true
    },
    "transfer": {
      "kind": "contract_seal",
      "sequence": 1,
      "attempt": 1,
      "validated": true
    }
  },
  "integrity": {
    "content_hash": "sha256:abc123...",
    "transfer_hash": "sha256:def456...",
    "chain_hash": "sha256:ghi789...",
    "signature": "optional_crypto_signature"
  },
  "payload": {
    "document": "...",
    "data": {}
  },
  "audit_trail": [
    {
      "event": "created",
      "timestamp": "2026-05-15T10:30:00Z",
      "by": "task_10"
    },
    {
      "event": "delivered",
      "timestamp": "2026-05-15T10:30:05Z",
      "by": "task_11",
      "hash_verified": true
    }
  ]
}
```

---

## 2. Componentes Chave

### 2.1 Metadata
- **id**: Identificador único do handoff (prefixo `hf_`)
- **version**: Versão do schema (semântica)
- **source**: Origem clara (task_id, role, timestamp)
- **destination**: Destino explícito (task_id, role, expected)
- **transfer**: Classificação da transferência (kind, sequence, attempt)

### 2.2 Integrity
- **content_hash**: SHA256 do payload (compactado)
- **transfer_hash**: Hash do link origem→destino (origem_id + destino_id + content_hash)
- **chain_hash**: Hash cumulativo (para verificar sequência)
- **signature**: Opcional, para assinatura criptográfica

### 2.3 Payload
- Dados da transferência (documento, JSON estruturado, etc)
- Compatível com estrutura anterior

### 2.4 Audit Trail
- Log de eventos imutável
- Rastreia criação, entrega, validação
- Timestamps e responsáveis

---

## 3. Tipos de Handoff

```
- contract_seal: Contrato selado pelo gerente
- task_plan: Plano de tarefas do arquiteto
- delivery: Entrega do operário
- audit_report: Relatório de auditoria
- knowledge_update: Atualização de conhecimento
- error_recovery: Recuperação de erro
```

---

## 4. Fluxo de Transferência

```
1. Tarefa Origem cria Handoff
   ├─ Estrutura envelope JSON
   ├─ Computa content_hash(payload)
   ├─ Computa transfer_hash(origem → destino)
   └─ Armazena no DB com status "CREATED"

2. Sistema localiza Tarefa Destino
   ├─ Valida task_id existe e está pronta
   ├─ Verifica se está na lista de dependências
   └─ Atualiza destination.expected = true

3. Tarefa Destino consome Handoff
   ├─ Valida content_hash(payload)
   ├─ Valida transfer_hash(origem → destino + content_hash)
   ├─ Registra no audit_trail com timestamp
   └─ Computa chain_hash (cumulativo de todas as transferências)

4. Validação de Integridade
   ├─ content_hash deve bater com payload
   ├─ transfer_hash = sha256(from_id + to_id + content_hash)
   └─ chain_hash rastreia sequência de handoffs
```

---

## 5. Compatibilidade com Banco de Dados

### Tabela atual (handoffs)
```sql
CREATE TABLE handoffs (
  id INTEGER PRIMARY KEY,
  contract_id INTEGER,
  from_task_id INTEGER,
  to_task_id INTEGER DEFAULT 0,
  kind TEXT,
  payload TEXT,
  sha256 TEXT,
  created_at DATETIME
);
```

### Campos mapeados para novo schema:
- `id` → metadata.id (migração para `hf_` prefixo)
- `from_task_id` → source.task_id
- `to_task_id` → destination.task_id
- `kind` → transfer.kind
- `payload` → payload (mantém JSON)
- `sha256` → integrity.content_hash
- `created_at` → metadata.source.timestamp

### Migração:
- Coluna nova: `transfer_hash TEXT`
- Coluna nova: `chain_hash TEXT`
- Coluna nova: `envelope JSONB` (serialização completa)
- Manter compatibilidade com leitura do payload

---

## 6. Exemplos de Uso

### Exemplo 1: Gerente selando contrato

```go
// Criar handoff
hf := handoff.New(
  "contract_seal",
  taskFrom: Task{ID: 10, Role: "gerente"},
  taskTo: Task{ID: 11, Role: "arquiteto"},
  payload: map[string]any{
    "contract_id": 42,
    "north_star": "...",
    "hash": "abc123",
  },
)

// Validar e computar hashes
if err := hf.Validate(); err != nil {
  return err
}

// Armazenar
store.AddHandoff(hf)
```

### Exemplo 2: Tarefa destino consumindo handoff

```go
// Recuperar handoff
hf, err := store.GetHandoff(hf_id)

// Validar integridade
if !hf.VerifyContentHash() {
  return errors.New("payload foi alterado")
}

if !hf.VerifyTransferHash() {
  return errors.New("transferência foi interceptada")
}

// Processar payload com confiança
data := hf.Payload
```

### Exemplo 3: Validar cadeia de handoffs

```go
// Rastrear sequência de transferências
chain := store.GetHandoffChain(contractID)

for i, hf := range chain {
  if i > 0 {
    prev := chain[i-1]
    if !hf.VerifyChainHash(prev.ChainHash) {
      return errors.New("cadeia de transferências quebrada")
    }
  }
}
```

---

## 7. Benefícios

✅ **JSON sempre**: Estrutura explícita  
✅ **Hash por transferência**: origem → destino validável  
✅ **Vínculo claro**: Sem ambiguidade (to_task_id=0 eliminado)  
✅ **Rastreabilidade**: Audit trail completo  
✅ **Validação**: Integridade garantida em cada ponto  
✅ **Compatibilidade**: DB retrô-compatível  
✅ **Observabilidade**: Hashes para debugging  

---

## 8. Implementação

### Fase 1: Core Types (pkg/handoff)
- `Handoff` struct
- `Metadata`, `Integrity`, `AuditEvent` structs
- Funções de hash

### Fase 2: Storage (database layer)
- Migrações DB
- CRUD methods
- Queries para rastreamento

### Fase 3: Integration
- Adaptar Manager para usar novo envelope
- Validação em AddHandoff
- Consumo em previousReports

### Fase 4: Testing & Docs
- Testes de integridade
- Exemplos
- Documentação
