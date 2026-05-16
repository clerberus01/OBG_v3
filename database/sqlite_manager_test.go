package database

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

func TestMigrateCopiesLegacyKnowledgeTable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "loja.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`CREATE TABLE conhecimento (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		topic TEXT NOT NULL UNIQUE,
		rules BLOB NOT NULL,
		expires_at DATETIME NOT NULL,
		last_used_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	);
	INSERT INTO conhecimento(topic, rules, expires_at) VALUES('legado', x'010203', datetime('now', '+1 hour'));`)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	item, err := store.GetKnowledge("legado")
	if err != nil {
		t.Fatal(err)
	}
	if item.Topic != "legado" || item.Format != "gob-rulemap" || len(item.Rules) != 3 {
		t.Fatalf("item = %#v", item)
	}
}

func TestMigrateCopiesLegacyHandoffsTable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "loja.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`CREATE TABLE contratos (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		north_star TEXT NOT NULL,
		constraints TEXT NOT NULL,
		deliverables TEXT NOT NULL,
		hash TEXT NOT NULL UNIQUE,
		sealed INTEGER NOT NULL DEFAULT 1,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	);
	CREATE TABLE tarefas (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		contract_id INTEGER NOT NULL,
		title TEXT NOT NULL,
		role TEXT NOT NULL,
		dependencies TEXT NOT NULL DEFAULT '[]',
		status TEXT NOT NULL,
		attempts INTEGER NOT NULL DEFAULT 0,
		read_only INTEGER NOT NULL DEFAULT 0,
		payload TEXT NOT NULL DEFAULT '{}',
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	);
	CREATE TABLE transferencias (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		contract_id INTEGER NOT NULL,
		from_task_id INTEGER NOT NULL,
		to_task_id INTEGER NOT NULL DEFAULT 0,
		kind TEXT NOT NULL,
		payload TEXT NOT NULL,
		sha256 TEXT NOT NULL,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	);
	INSERT INTO contratos(id, north_star, constraints, deliverables, hash) VALUES(1, 'n', 'c', 'd', 'h');
	INSERT INTO tarefas(id, contract_id, title, role, status) VALUES(2, 1, 't', 'r', 'Aprovado');
	INSERT INTO transferencias(contract_id, from_task_id, kind, payload, sha256) VALUES(1, 2, 'handoff', '{"ok":true}', 'abc');`)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	handoffs, err := store.ListHandoffs(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(handoffs) != 1 || handoffs[0].Kind != "handoff" || handoffs[0].Payload["ok"] != true {
		t.Fatalf("handoffs = %#v", handoffs)
	}
}

func TestPluginCallHistoryStoresStructuredMetadata(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "loja.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	err = store.AddPluginCallRecord(PluginCallRecord{
		PluginID:   "svc",
		Tool:       "echo",
		Transport:  "local-service",
		ContractID: 10,
		TaskID:     20,
		Input:      map[string]any{"value": "ok"},
		Output:     `{"ok":true}`,
		OK:         true,
		Duration:   "5ms",
		Sandboxed:  true,
		WorkDir:    `.sandbox\svc\contract-10\task-20`,
	})
	if err != nil {
		t.Fatal(err)
	}
	calls, err := store.ListPluginCalls(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(calls) != 1 {
		t.Fatalf("calls = %#v", calls)
	}
	call := calls[0]
	if call.PluginID != "svc" || call.Tool != "echo" || call.Transport != "local-service" {
		t.Fatalf("call identity = %#v", call)
	}
	if call.ContractID != 10 || call.TaskID != 20 || !call.Sandboxed || call.WorkDir == "" || !call.OK {
		t.Fatalf("call metadata = %#v", call)
	}
	if !strings.Contains(call.Input, `"value":"ok"`) || call.Output != `{"ok":true}` {
		t.Fatalf("call payload = %#v", call)
	}
}

func TestPluginCommandRegistryPersistsLocalAndWebCommands(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "loja.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	err = store.UpsertPluginCommandRegistry([]PluginCommandRegistration{
		{
			PluginID:     "local",
			Tool:         "format",
			Kind:         "local-command",
			Transport:    "stdio-json",
			Target:       "gofmt -w",
			Enabled:      true,
			ManifestPath: "plugins/local.json",
			Sandbox:      `{"max_output_bytes":16000}`,
		},
		{
			PluginID:     "web",
			Tool:         "send",
			Kind:         "web-service",
			Transport:    "web-service",
			Target:       "https://example.com/tool",
			Enabled:      false,
			ManifestPath: "plugins/web.json",
			Sandbox:      `{}`,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	items, err := store.ListPluginCommandRegistry(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("items = %#v", items)
	}
	found := map[string]PluginCommandRegistration{}
	for _, item := range items {
		found[item.PluginID+"/"+item.Tool] = item
	}
	if !found["local/format"].Enabled || found["local/format"].Kind != "local-command" || found["local/format"].Target != "gofmt -w" {
		t.Fatalf("local registry = %#v", found["local/format"])
	}
	if found["web/send"].Enabled || found["web/send"].Kind != "web-service" || found["web/send"].Target != "https://example.com/tool" {
		t.Fatalf("web registry = %#v", found["web/send"])
	}
}

func TestPluginPermissionScopesResolveTaskBeforeContract(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "loja.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.UpsertPluginPermissionScope(PluginPermissionScope{
		Scope:       "contract",
		ContractID:  10,
		PluginID:    "echo",
		Tool:        "*",
		Permissions: `{"allow_commands":["go test"]}`,
		Enabled:     true,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertPluginPermissionScope(PluginPermissionScope{
		Scope:       "task",
		ContractID:  10,
		TaskID:      20,
		PluginID:    "echo",
		Tool:        "call",
		Permissions: `{"allow_commands":["gofmt"]}`,
		Enabled:     true,
	}); err != nil {
		t.Fatal(err)
	}
	scope, ok, err := store.PluginPermissionsForCall(10, 20, "echo", "call")
	if err != nil {
		t.Fatal(err)
	}
	if !ok || scope.Scope != "task" || scope.TaskID != 20 || !strings.Contains(scope.Permissions, "gofmt") {
		t.Fatalf("scope = %#v ok=%v", scope, ok)
	}
	scopes, err := store.ListPluginPermissionScopes(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(scopes) != 2 {
		t.Fatalf("scopes = %#v", scopes)
	}
}

func TestPluginCallMigrationAddsStructuredColumns(t *testing.T) {
	path := filepath.Join(t.TempDir(), "loja.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`CREATE TABLE plugin_calls (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		plugin_id TEXT NOT NULL,
		tool TEXT NOT NULL,
		input TEXT NOT NULL,
		output TEXT NOT NULL,
		ok INTEGER NOT NULL,
		duration TEXT NOT NULL,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	);
	INSERT INTO plugin_calls(plugin_id, tool, input, output, ok, duration) VALUES('old', 'echo', '{}', '{}', 1, '1ms');`)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	calls, err := store.ListPluginCalls(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(calls) != 1 || calls[0].PluginID != "old" {
		t.Fatalf("legacy call missing: %#v", calls)
	}
	if calls[0].Transport != "" || calls[0].ContractID != 0 || calls[0].Sandboxed {
		t.Fatalf("legacy defaults not applied: %#v", calls[0])
	}
}

func TestHandoffStoresJSONMetadataAndTransferHash(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "loja.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	contract, err := store.CreateContract("north", "constraints", "deliverables")
	if err != nil {
		t.Fatal(err)
	}
	source, err := store.AddTask(contract.ID, "source", "Gerente", nil)
	if err != nil {
		t.Fatal(err)
	}
	targetA, err := store.AddTask(contract.ID, "target a", "Auditor", []int64{source.ID})
	if err != nil {
		t.Fatal(err)
	}
	targetB, err := store.AddTask(contract.ID, "target b", "Auditor", []int64{source.ID})
	if err != nil {
		t.Fatal(err)
	}
	first, err := store.AddHandoff(contract.ID, source.ID, targetA.ID, "handoff", map[string]any{"report": "ok"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.AddHandoff(contract.ID, source.ID, targetB.ID, "handoff", map[string]any{"report": "ok"})
	if err != nil {
		t.Fatal(err)
	}
	if first.SHA256 == second.SHA256 {
		t.Fatalf("handoff hash must include transfer destination: %s", first.SHA256)
	}
	meta, ok := first.Payload["handoff"].(map[string]any)
	if !ok {
		t.Fatalf("handoff metadata missing: %#v", first.Payload)
	}
	if meta["from_task_id"].(float64) != float64(source.ID) || meta["to_task_id"].(float64) != float64(targetA.ID) {
		t.Fatalf("bad handoff metadata: %#v", meta)
	}
	if first.Payload["report"] != "ok" {
		t.Fatalf("original payload should remain at top level: %#v", first.Payload)
	}
}

func TestContractIsImmutableWhenSealed(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "loja.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	contract, err := store.CreateContract("north", "constraints", "deliverables")
	if err != nil {
		t.Fatal(err)
	}
	if !contract.Sealed {
		t.Fatal("contract should be sealed by default")
	}
	_, err = store.db.Exec(`UPDATE contratos SET north_star = 'mutado' WHERE id = ?`, contract.ID)
	if err == nil || !strings.Contains(err.Error(), "contrato imutavel") {
		t.Fatalf("expected immutable update rejection, got %v", err)
	}
	_, err = store.db.Exec(`DELETE FROM contratos WHERE id = ?`, contract.ID)
	if err == nil || !strings.Contains(err.Error(), "contrato imutavel") {
		t.Fatalf("expected immutable delete rejection, got %v", err)
	}
	loaded, err := store.GetContract(contract.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Hash != contract.Hash || loaded.NorthStar != "north" {
		t.Fatalf("contract mutated: %#v", loaded)
	}
}

func TestApprovedTaskIsReadOnly(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "loja.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	contract, err := store.CreateContract("north", "constraints", "deliverables")
	if err != nil {
		t.Fatal(err)
	}
	task, err := store.AddTask(contract.ID, "do work", "Worker", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.MarkRunning(task.ID); err != nil {
		t.Fatal(err)
	}
	if err := store.ApproveTask(task.ID, map[string]any{"ok": true}); err != nil {
		t.Fatal(err)
	}
	approved, err := store.GetTask(task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !approved.ReadOnly || approved.Status != StatusApproved {
		t.Fatalf("task not sealed read-only: %#v", approved)
	}
	if err := store.MarkRunning(task.ID); err == nil {
		t.Fatal("read-only task should not be marked running")
	}
	if err := store.RetryTask(task.ID, "retry"); err == nil {
		t.Fatal("read-only task should not be retried")
	}
}

func TestThreeFailuresBlockTask(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "loja.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	contract, err := store.CreateContract("north", "constraints", "deliverables")
	if err != nil {
		t.Fatal(err)
	}
	task, err := store.AddTask(contract.ID, "do work", "Worker", nil)
	if err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 3; i++ {
		if err := store.RejectTask(task.ID, "falha"); err != nil {
			t.Fatal(err)
		}
		current, err := store.GetTask(task.ID)
		if err != nil {
			t.Fatal(err)
		}
		if i < 3 && current.Status != StatusRedoing {
			t.Fatalf("attempt %d status = %s", i, current.Status)
		}
		if i == 3 {
			if current.Status != StatusBlocked || current.Attempts != 3 {
				t.Fatalf("task should be blocked after 3 failures: %#v", current)
			}
		}
		if current.Payload["reason"] != "falha" {
			t.Fatalf("failure reason not stored: %#v", current.Payload)
		}
		watchdog, ok := current.Payload["watchdog"].(map[string]any)
		if !ok {
			t.Fatalf("watchdog payload missing: %#v", current.Payload)
		}
		if int(watchdog["attempt"].(float64)) != i || watchdog["reason"] != "falha" {
			t.Fatalf("watchdog payload = %#v", watchdog)
		}
	}
}
