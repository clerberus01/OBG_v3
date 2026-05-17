package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"omni-bot-go/database"
	"omni-bot-go/engine"
	"omni-bot-go/knowledge"
	"omni-bot-go/mcp"
)

func TestRuntimeHealthOK(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	dbPath := filepath.Join("data", "loja.db")
	logPath := filepath.Join("logs", "omni-bot-go.log")
	if err := ensureRuntimeDirs(dbPath, logPath, "plugins"); err != nil {
		t.Fatal(err)
	}
	store, err := database.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	eng, err := engine.New()
	if err != nil {
		t.Fatal(err)
	}
	manager := NewManager(store, eng, knowledge.New(store), NewEventBus())
	health := runtimeHealth(manager, dbPath, "plugins", logPath)
	if health["status"] != "ok" {
		t.Fatalf("health = %#v", health)
	}
}

func TestSnapshotExposesEngineWithoutModelKey(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	store, err := database.Open(filepath.Join(root, "loja.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	eng, err := engine.New()
	if err != nil {
		t.Fatal(err)
	}
	manager := NewManager(store, eng, knowledge.New(store), NewEventBus())
	snapshot := manager.Snapshot()
	if _, ok := snapshot["engine"]; !ok {
		t.Fatalf("snapshot sem chave engine: %#v", snapshot)
	}
	if _, ok := snapshot["model"]; ok {
		t.Fatalf("snapshot nao deve expor chave model: %#v", snapshot["model"])
	}
}

func TestInterrogationPreviewAndFinalizeUsesReviewedDraft(t *testing.T) {
	store, err := database.Open(filepath.Join(t.TempDir(), "loja.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	eng, err := engine.New()
	if err != nil {
		t.Fatal(err)
	}
	manager := NewManager(store, eng, knowledge.New(store), NewEventBus())
	session, err := manager.StartInterrogation("automatizar auditoria de contratos")
	if err != nil {
		t.Fatal(err)
	}
	answers := map[string]string{
		"q1": "Contrato auditavel revisado antes de selar",
		"q2": "Go, SQLite e dashboard",
		"q3": "Sem modelos externos em producao",
		"q4": "go test ./...",
	}
	draft, err := manager.PreviewInterrogationContract(session.ID, answers)
	if err != nil {
		t.Fatal(err)
	}
	if draft.Hash == "" || draft.NorthStar == "" {
		t.Fatalf("draft incompleto: %#v", draft)
	}
	contract, err := manager.FinalizeInterrogationWithDraft(session.ID, answers, ContractDraft{
		NorthStar:    "Objetivo revisado manualmente",
		Constraints:  "Restricao revisada manualmente",
		Deliverables: "Entrega revisada manualmente",
	})
	if err != nil {
		t.Fatal(err)
	}
	if contract.NorthStar != "Objetivo revisado manualmente" || contract.Constraints != "Restricao revisada manualmente" || contract.Deliverables != "Entrega revisada manualmente" {
		t.Fatalf("contract nao usou draft revisado: %#v", contract)
	}
}

func TestWorkersAreSeparatedByTaskRoleWithoutExtraQueues(t *testing.T) {
	store, err := database.Open(filepath.Join(t.TempDir(), "loja.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	eng, err := engine.New()
	if err != nil {
		t.Fatal(err)
	}
	manager := NewManager(store, eng, knowledge.New(store), NewEventBus())
	if got := manager.applicationGoroutineBudget(); got != 5 {
		t.Fatalf("application goroutine budget = %d want 5", got)
	}
	if len(manager.workerQueues) != 4 {
		t.Fatalf("worker queues = %d", len(manager.workerQueues))
	}
	cases := map[string]int{
		"Gerente":              1,
		"Arquiteto":            1,
		"Desenvolvedor":        2,
		"Operario Polimorfico": 2,
		"Analista":             3,
		"Redator":              3,
		"Designer de Produto":  3,
		"Auditor Tecnico":      4,
	}
	for role, want := range cases {
		if got := workerIDForTaskRole(role); got != want {
			t.Fatalf("role %q routed to worker %d want %d", role, got, want)
		}
	}
	agents, err := store.ListAgents()
	if err != nil {
		t.Fatal(err)
	}
	if len(agents) != 4 {
		t.Fatalf("agents = %#v", agents)
	}
	for _, agent := range agents {
		if agent.Role != workerIdleRole(agent.WorkerID) {
			t.Fatalf("agent role = %#v", agent)
		}
	}
}

func TestFactorySeriesCreatesSerialContracts(t *testing.T) {
	store, err := database.Open(filepath.Join(t.TempDir(), "loja.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	eng, err := engine.New()
	if err != nil {
		t.Fatal(err)
	}
	manager := NewManager(store, eng, knowledge.New(store), NewEventBus())
	result, err := manager.CreateFactorySeries(FactorySeriesRequest{
		Template: "Gerar documento tecnico para {{item}}",
		Items:    []string{"Modulo A", "Modulo B"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Mode != "factory-series" || result.Count != 2 || len(result.Contracts) != 2 || result.BatchID == "" {
		t.Fatalf("factory result = %#v", result)
	}
	batches, err := store.ListFactoryBatches(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(batches) != 1 || batches[0].BatchID != result.BatchID || batches[0].Total != 2 || batches[0].Status != "active" {
		t.Fatalf("factory batches = %#v", batches)
	}
	items, err := store.ListAllFactoryItems(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("factory items = %#v", items)
	}
	for _, item := range items {
		if item.BatchID != result.BatchID || item.ContractID == 0 || item.Status != "created" {
			t.Fatalf("factory item incompleto: %#v", item)
		}
	}
	tasks, err := store.ListTasks()
	if err != nil {
		t.Fatal(err)
	}
	var firstFinal, secondFirst database.Task
	for _, task := range tasks {
		factory, ok := task.Payload["factory"].(map[string]any)
		if !ok || factory["batch_id"] != result.BatchID {
			continue
		}
		if task.ContractID == result.Contracts[0].ID {
			firstFinal = task
		}
		if task.ContractID == result.Contracts[1].ID && len(secondFirst.Dependencies) == 0 {
			secondFirst = task
		}
		if task.Payload["source"] != "factory_series" {
			t.Fatalf("task without factory source: %#v", task.Payload)
		}
	}
	if firstFinal.ID == 0 || secondFirst.ID == 0 {
		t.Fatalf("factory tasks not found: %#v", tasks)
	}
	if len(secondFirst.Dependencies) != 1 || secondFirst.Dependencies[0] != firstFinal.ID {
		t.Fatalf("second contract first task must depend on previous final task: first=%#v second=%#v", firstFinal, secondFirst)
	}
	snapshot := manager.Snapshot()
	factoryItems, ok := snapshot["factory_series"].([]factorySeriesSummary)
	if !ok || len(factoryItems) != 1 || factoryItems[0].BatchID != result.BatchID || factoryItems[0].Contracts != 2 {
		t.Fatalf("snapshot factory = %#v", snapshot["factory_series"])
	}
	snapshotBatches, ok := snapshot["factory_batches"].([]database.FactoryBatch)
	if !ok || len(snapshotBatches) != 1 || snapshotBatches[0].BatchID != result.BatchID {
		t.Fatalf("snapshot factory_batches = %#v", snapshot["factory_batches"])
	}
	snapshotItems, ok := snapshot["factory_items"].([]database.FactoryItem)
	if !ok || len(snapshotItems) != 2 {
		t.Fatalf("snapshot factory_items = %#v", snapshot["factory_items"])
	}
	contractSummaries, ok := snapshot["contract_summaries"].([]contractDashboardSummary)
	if !ok || len(contractSummaries) != 2 {
		t.Fatalf("snapshot contract_summaries = %#v", snapshot["contract_summaries"])
	}
	for _, item := range contractSummaries {
		if item.Tasks == 0 || item.FactoryBatchID != result.BatchID || item.FactoryIndex == 0 || item.FactoryTotal != 2 {
			t.Fatalf("contract summary incompleto: %#v", item)
		}
		if len(item.Domains) == 0 || len(item.Roles) == 0 {
			t.Fatalf("contract summary sem dominio/papel: %#v", item)
		}
	}
	edges, ok := snapshot["task_dependencies"].([]taskDependencyEdge)
	if !ok || len(edges) == 0 {
		t.Fatalf("snapshot task_dependencies = %#v", snapshot["task_dependencies"])
	}
	var foundCrossContract bool
	for _, edge := range edges {
		if edge.FromTaskID == firstFinal.ID && edge.ToTaskID == secondFirst.ID && edge.CrossContract {
			foundCrossContract = true
		}
	}
	if !foundCrossContract {
		t.Fatalf("dependencia entre contratos nao exposta: %#v", edges)
	}
	if got := manager.applicationGoroutineBudget(); got != 5 {
		t.Fatalf("factory must not add goroutines, got %d", got)
	}
}

func TestFactoryBatchOperationalActions(t *testing.T) {
	store, err := database.Open(filepath.Join(t.TempDir(), "loja.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	eng, err := engine.New()
	if err != nil {
		t.Fatal(err)
	}
	manager := NewManager(store, eng, knowledge.New(store), NewEventBus())
	result, err := manager.CreateFactorySeries(FactorySeriesRequest{
		Template: "Gerar documento tecnico para {{item}}",
		Items:    []string{"Modulo A", "Modulo B"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.FactoryBatchAction(FactoryBatchActionRequest{BatchID: result.BatchID, Action: "pause", Reason: "janela operacional"}); err != nil {
		t.Fatal(err)
	}
	batch, err := store.GetFactoryBatch(result.BatchID)
	if err != nil {
		t.Fatal(err)
	}
	if batch.Status != "paused" {
		t.Fatalf("batch paused = %#v", batch)
	}
	if _, err := manager.FactoryBatchAction(FactoryBatchActionRequest{BatchID: result.BatchID, Action: "resume", Reason: "retomar"}); err != nil {
		t.Fatal(err)
	}
	batch, _ = store.GetFactoryBatch(result.BatchID)
	if batch.Status != "active" {
		t.Fatalf("batch resumed = %#v", batch)
	}
	if _, err := manager.FactoryBatchAction(FactoryBatchActionRequest{BatchID: result.BatchID, Action: "skip", Index: 1, Reason: "fora do lote"}); err != nil {
		t.Fatal(err)
	}
	item, err := store.GetFactoryItem(result.BatchID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if item.Status != "skipped" {
		t.Fatalf("skipped item = %#v", item)
	}
	if _, err := manager.FactoryBatchAction(FactoryBatchActionRequest{BatchID: result.BatchID, Action: "reprocess", Index: 2, Reason: "nova auditoria"}); err != nil {
		t.Fatal(err)
	}
	item, _ = store.GetFactoryItem(result.BatchID, 2)
	if item.Status != "reprocess_requested" {
		t.Fatalf("reprocess item = %#v", item)
	}
	tasks, err := store.ListTasks()
	if err != nil {
		t.Fatal(err)
	}
	var reauditFound bool
	for _, task := range tasks {
		if task.ContractID == result.Contracts[1].ID && task.Role == "Auditor Tecnico" && strings.Contains(task.Title, "Reexecutar auditoria") {
			reauditFound = true
		}
	}
	if !reauditFound {
		t.Fatalf("reauditoria do item nao criada: %#v", tasks)
	}
	actionResult, err := manager.FactoryBatchAction(FactoryBatchActionRequest{BatchID: result.BatchID, Action: "cancel", Reason: "cancelamento de lote"})
	if err != nil {
		t.Fatal(err)
	}
	if actionResult["cancelled_tasks"] == nil {
		t.Fatalf("cancel result = %#v", actionResult)
	}
	batch, _ = store.GetFactoryBatch(result.BatchID)
	if batch.Status != "cancelled" {
		t.Fatalf("batch cancelled = %#v", batch)
	}
}

func TestFactorySeriesIdempotencyAndResumeState(t *testing.T) {
	store, err := database.Open(filepath.Join(t.TempDir(), "loja.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	eng, err := engine.New()
	if err != nil {
		t.Fatal(err)
	}
	manager := NewManager(store, eng, knowledge.New(store), NewEventBus())
	req := FactorySeriesRequest{
		Template: "Gerar documento tecnico para {{item}}",
		Items:    []string{"Modulo A", "Modulo B"},
	}
	first, err := manager.CreateFactorySeries(req)
	if err != nil {
		t.Fatal(err)
	}
	firstTasks, err := store.ListTasks()
	if err != nil {
		t.Fatal(err)
	}
	second, err := manager.CreateFactorySeries(req)
	if err != nil {
		t.Fatal(err)
	}
	if second.BatchID != first.BatchID || len(second.Contracts) != len(first.Contracts) {
		t.Fatalf("idempotent result mismatch: first=%#v second=%#v", first, second)
	}
	contracts, err := store.ListContracts()
	if err != nil {
		t.Fatal(err)
	}
	if len(contracts) != 2 {
		t.Fatalf("repetir lote nao deve duplicar contratos: %#v", contracts)
	}
	tasks, err := store.ListTasks()
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != len(firstTasks) {
		t.Fatalf("repetir lote nao deve duplicar tarefas: before=%d after=%d", len(firstTasks), len(tasks))
	}
	forced, err := manager.CreateFactorySeries(FactorySeriesRequest{
		Template: "Gerar documento tecnico para {{item}}",
		Items:    []string{"Modulo A", "Modulo B"},
		ForceNew: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if forced.BatchID == first.BatchID || len(forced.Contracts) != 2 {
		t.Fatalf("force_new deve criar lote separado: first=%#v forced=%#v", first, forced)
	}
	if _, err := manager.FactoryBatchAction(FactoryBatchActionRequest{BatchID: first.BatchID, Action: "pause", Reason: "teste de retomada"}); err != nil {
		t.Fatal(err)
	}
	reloadedEngine, err := engine.New()
	if err != nil {
		t.Fatal(err)
	}
	reloaded := NewManager(store, reloadedEngine, knowledge.New(store), NewEventBus())
	var pausedTask database.Task
	for _, task := range tasks {
		if task.ContractID == first.Contracts[0].ID {
			pausedTask = task
			break
		}
	}
	if pausedTask.ID == 0 {
		t.Fatalf("tarefa do lote pausado nao encontrada")
	}
	if !reloaded.taskFactoryBatchBlocked(pausedTask) {
		t.Fatalf("manager reiniciado deve preservar bloqueio do lote pausado")
	}
}

func TestWatchdogFailurePublishesEventAndBlocksAfterThreeAttempts(t *testing.T) {
	store, err := database.Open(filepath.Join(t.TempDir(), "loja.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	eng, err := engine.New()
	if err != nil {
		t.Fatal(err)
	}
	bus := NewEventBus()
	manager := NewManager(store, eng, knowledge.New(store), bus)
	ch := bus.Subscribe()
	defer bus.Unsubscribe(ch)
	contract, err := store.CreateContract("north", "constraints", "deliverables")
	if err != nil {
		t.Fatal(err)
	}
	task, err := store.AddTask(contract.ID, "falhar tarefa", "Desenvolvedor", nil)
	if err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 3; i++ {
		manager.watchdogFailure(task, "erro controlado")
	}
	var last map[string]any
	for i := 1; i <= 3; i++ {
		var msg map[string]any
		select {
		case raw := <-ch:
			if err := json.Unmarshal(raw, &msg); err != nil {
				t.Fatal(err)
			}
			last = msg
		default:
			t.Fatalf("missing watchdog event %d", i)
		}
	}
	if last["kind"] != "watchdog" {
		t.Fatalf("event = %#v", last)
	}
	payload := last["payload"].(map[string]any)
	if payload["reason"] != "erro controlado" || payload["blocked"] != true || int(payload["attempt"].(float64)) != 3 {
		t.Fatalf("payload = %#v", payload)
	}
	blocked, err := store.GetTask(task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if blocked.Status != database.StatusBlocked || blocked.Attempts != 3 {
		t.Fatalf("task = %#v", blocked)
	}
	watchdog := blocked.Payload["watchdog"].(map[string]any)
	if watchdog["reason"] != "erro controlado" || watchdog["blocked"] != true {
		t.Fatalf("watchdog payload = %#v", watchdog)
	}
	snapshot := manager.Snapshot()
	events, ok := snapshot["watchdog_events"].([]watchdogSnapshotEvent)
	if !ok || len(events) != 1 {
		t.Fatalf("snapshot watchdog_events = %#v", snapshot["watchdog_events"])
	}
	if events[0].TaskID != task.ID || !events[0].Blocked || events[0].Attempt != 3 || events[0].Reason != "erro controlado" {
		t.Fatalf("watchdog snapshot = %#v", events[0])
	}
}

func TestPluginCallCannotUseReadOnlyTaskScope(t *testing.T) {
	store, err := database.Open(filepath.Join(t.TempDir(), "loja.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	eng, err := engine.New()
	if err != nil {
		t.Fatal(err)
	}
	manager := NewManager(store, eng, knowledge.New(store), NewEventBus())
	contract, err := store.CreateContract("north", "constraints", "deliverables")
	if err != nil {
		t.Fatal(err)
	}
	task, err := store.AddTask(contract.ID, "tarefa selada", "Desenvolvedor", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.MarkRunning(task.ID); err != nil {
		t.Fatal(err)
	}
	if err := store.ApproveTask(task.ID, map[string]any{"ok": true}); err != nil {
		t.Fatal(err)
	}
	_, err = manager.CallPlugin(mcp.CallRequest{PluginID: "fixture", Tool: "echo", TaskID: task.ID})
	if err == nil || err.Error() != "tarefa read-only nao pode executar plugin" {
		t.Fatalf("expected read-only task error, got %v", err)
	}
}

func TestCombinePluginPermissionsIsRestrictive(t *testing.T) {
	combined := combinePluginPermissions(mcp.SandboxPolicy{
		WorkDir:        "contract",
		AllowCommands:  []string{"go test", "gofmt"},
		AllowEnv:       []string{"A", "B"},
		MaxOutputBytes: 1000,
	}, mcp.SandboxPolicy{
		WorkDir:        "task",
		AllowCommands:  []string{"gofmt"},
		AllowEnv:       []string{"B", "C"},
		MaxOutputBytes: 500,
	})
	if combined.WorkDir != filepath.Join("contract", "task") {
		t.Fatalf("workdir = %q", combined.WorkDir)
	}
	if len(combined.AllowCommands) != 1 || combined.AllowCommands[0] != "gofmt" {
		t.Fatalf("commands = %#v", combined.AllowCommands)
	}
	if len(combined.AllowEnv) != 1 || combined.AllowEnv[0] != "B" {
		t.Fatalf("env = %#v", combined.AllowEnv)
	}
	if combined.MaxOutputBytes != 500 {
		t.Fatalf("max output = %d", combined.MaxOutputBytes)
	}
}

func TestAssertivenessMetricsByRoleAndDomain(t *testing.T) {
	base := time.Date(2026, 5, 15, 10, 0, 0, 0, time.UTC)
	tasks := []database.Task{
		{
			Role:      "Desenvolvedor",
			Status:    database.StatusApproved,
			Payload:   map[string]any{"domain": "code"},
			CreatedAt: base,
			UpdatedAt: base.Add(10 * time.Second),
		},
		{
			Role:      "Desenvolvedor",
			Status:    database.StatusBlocked,
			Attempts:  3,
			Payload:   map[string]any{"domain": "code"},
			CreatedAt: base,
			UpdatedAt: base.Add(20 * time.Second),
		},
		{
			Role:      "Auditor Tecnico",
			Status:    database.StatusApproved,
			Attempts:  1,
			Payload:   map[string]any{"domain": "audit"},
			CreatedAt: base,
			UpdatedAt: base.Add(30 * time.Second),
		},
		{
			Role:      "Gerente",
			Status:    database.StatusRunning,
			Payload:   map[string]any{"domain": "general"},
			CreatedAt: base,
			UpdatedAt: base.Add(40 * time.Second),
		},
	}
	metrics := buildAssertivenessMetrics(tasks)
	if metrics.Approved != 2 || metrics.Redone != 2 || metrics.Blocked != 1 {
		t.Fatalf("totals = %#v", metrics)
	}
	if metrics.AverageSeconds != 20 {
		t.Fatalf("average = %v", metrics.AverageSeconds)
	}
	code := findRate(metrics.SuccessRateByDomain, "code")
	if code.Total != 2 || code.Approved != 1 || code.Failed != 1 || code.SuccessRate != 0.5 {
		t.Fatalf("code = %#v", code)
	}
	dev := findRate(metrics.SuccessRateByRole, "Desenvolvedor")
	if dev.Total != 2 || dev.SuccessRate != 0.5 {
		t.Fatalf("dev = %#v", dev)
	}
	auditor := findRate(metrics.SuccessRateByRole, "Auditor Tecnico")
	if auditor.Total != 1 || auditor.SuccessRate != 1 {
		t.Fatalf("auditor = %#v", auditor)
	}
}

func findRate(items []successRateItem, key string) successRateItem {
	for _, item := range items {
		if item.Key == key {
			return item
		}
	}
	return successRateItem{}
}

func TestJITKnowledgePackPersistsAndReloads(t *testing.T) {
	root := t.TempDir()
	dbPath := filepath.Join(root, "loja.db")
	store, err := database.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	lib := knowledge.New(store)
	if _, err := lib.IngestText("pack jit sqlite", "test", `
- Deve persistir regras JIT em knowledge_packs.
- Sempre recarregar packs persistidos no startup.
`); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := database.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	eng, err := engine.New()
	if err != nil {
		t.Fatal(err)
	}
	manager := NewManager(reopened, eng, knowledge.New(reopened), NewEventBus())
	info := manager.engine.Info()
	if info.PackCount < 2 {
		t.Fatalf("expected core + persisted JIT pack, got %#v", info)
	}
	found := false
	for _, pack := range info.Packs {
		if pack.Domain == "pack jit sqlite" {
			found = true
		}
	}
	if !found {
		t.Fatalf("persisted JIT pack not reloaded: %#v", info.Packs)
	}
}

func TestStartupLoadsDiskKnowledgePacks(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	if err := os.MkdirAll("knowledge", 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join("knowledge", "startup.pack.json"), []byte(`{
  "domain": "startup",
  "version": "2.0",
  "rules": [
    {
      "pattern": "startup",
      "action": "disk_pack_loaded",
      "template": "Aplicar regra carregada do disco para {{.Description}}."
    }
  ]
}`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join("knowledge", "broken.pack.json"), []byte(`{"domain":`), 0644); err != nil {
		t.Fatal(err)
	}
	store, err := database.Open(filepath.Join(root, "loja.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	eng, err := engine.New()
	if err != nil {
		t.Fatal(err)
	}
	manager := NewManager(store, eng, knowledge.New(store), NewEventBus())
	if loaded := manager.LoadDiskKnowledgePacks("knowledge"); loaded != 1 {
		t.Fatalf("loaded = %d", loaded)
	}
	info := manager.engine.Info()
	found := false
	for _, pack := range info.Packs {
		if pack.Domain == "startup" && pack.RuleCount == 1 {
			found = true
		}
	}
	if !found {
		t.Fatalf("disk pack not loaded: %#v", info.Packs)
	}
	persisted, err := manager.knowledgePacks.Load("startup")
	if err != nil {
		t.Fatal(err)
	}
	if persisted.Source == "" || len(persisted.PackRules) != 1 || persisted.PackRules[0].Pattern != "startup" {
		t.Fatalf("disk pack not persisted with native rules: %#v", persisted)
	}
	reloadedEngine, err := engine.New()
	if err != nil {
		t.Fatal(err)
	}
	reloadedManager := NewManager(store, reloadedEngine, knowledge.New(store), NewEventBus())
	reloadedInfo := reloadedManager.engine.Info()
	found = false
	for _, pack := range reloadedInfo.Packs {
		if pack.Domain == "startup" && pack.RuleCount == 1 {
			found = true
		}
	}
	if !found {
		t.Fatalf("persisted disk pack not reloaded from sqlite: %#v", reloadedInfo.Packs)
	}
}

func TestAPIRuntimeEndpoints(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	dbPath := filepath.Join("data", "loja.db")
	logPath := filepath.Join("logs", "omni-bot-go.log")
	if err := ensureRuntimeDirs(dbPath, logPath, "plugins"); err != nil {
		t.Fatal(err)
	}
	store, err := database.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	eng, err := engine.New()
	if err != nil {
		t.Fatal(err)
	}
	manager := NewManager(store, eng, knowledge.New(store), NewEventBus())
	mux := runtimeTestMux(manager, eng, dbPath, "plugins", logPath)

	for _, tc := range []struct {
		path string
		key  string
	}{
		{path: "/api/health", key: "status"},
		{path: "/api/engine", key: "mode"},
		{path: "/api/snapshot", key: "engine"},
	} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, tc.path, nil)
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s status = %d body=%s", tc.path, rec.Code, rec.Body.String())
		}
		var body map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("%s invalid json: %v", tc.path, err)
		}
		if _, ok := body[tc.key]; !ok {
			t.Fatalf("%s response missing %q: %#v", tc.path, tc.key, body)
		}
	}
}

func TestAPIFactorySeriesAndKnowledgeCandidateActions(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	store, err := database.Open(filepath.Join(root, "loja.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	eng, err := engine.New()
	if err != nil {
		t.Fatal(err)
	}
	manager := NewManager(store, eng, knowledge.New(store), NewEventBus())
	mux := runtimeTestMux(manager, eng, "loja.db", "plugins", "logs/test.log")

	body := `{"template":"Gerar documento tecnico para {{item}}","items":["A","B"]}`
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/factory_series", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("factory status = %d body=%s", rec.Code, rec.Body.String())
	}
	var factory FactorySeriesResult
	if err := json.Unmarshal(rec.Body.Bytes(), &factory); err != nil {
		t.Fatal(err)
	}
	if factory.BatchID == "" || factory.Count != 2 || len(factory.Contracts) != 2 {
		t.Fatalf("factory = %#v", factory)
	}
	rec = httptest.NewRecorder()
	actionBody := fmt.Sprintf(`{"batch_id":%q,"action":"pause","reason":"teste api"}`, factory.BatchID)
	req = httptest.NewRequest(http.MethodPost, "/api/factory_batches/action", strings.NewReader(actionBody))
	req.Header.Set("Content-Type", "application/json")
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("factory action status = %d body=%s", rec.Code, rec.Body.String())
	}
	batch, err := store.GetFactoryBatch(factory.BatchID)
	if err != nil {
		t.Fatal(err)
	}
	if batch.Status != "paused" {
		t.Fatalf("batch after api action = %#v", batch)
	}

	task, err := store.AddTaskWithPayload(factory.Contracts[0].ID, "Implementar codigo auditavel", "Desenvolvedor", nil, map[string]any{"domain": "code", "key": "code-implement"})
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.createKnowledgeCandidate(task, map[string]any{
		"artifacts": []fileArtifact{{Path: "projects/contract-1/api.json", SHA256: "abc", Bytes: 2}},
	}); err != nil {
		t.Fatal(err)
	}
	pending, err := manager.KnowledgeCandidateSummaries("pending", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 1 {
		t.Fatalf("pending = %#v", pending)
	}
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/knowledge_candidates", strings.NewReader(fmt.Sprintf(`{"id":%d,"action":"reject"}`, pending[0].ID)))
	req.Header.Set("Content-Type", "application/json")
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("candidate status = %d body=%s", rec.Code, rec.Body.String())
	}
	rejected, err := manager.KnowledgeCandidateSummaries("rejected", 10)
	if err != nil || len(rejected) != 1 {
		t.Fatalf("rejected=%#v err=%v", rejected, err)
	}
}

func TestAPIKnowledgePacksSearchFilters(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	store, err := database.Open(filepath.Join(root, "loja.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	eng, err := engine.New()
	if err != nil {
		t.Fatal(err)
	}
	lib := knowledge.New(store)
	if err := lib.SaveKnowledgePack(&knowledge.KnowledgePack{
		Domain:  "code",
		Version: "2.0",
		Rules: []knowledge.Rule{
			{Pattern: "api", Action: "contract_api", Template: "Validar contrato de API."},
		},
	}, "test"); err != nil {
		t.Fatal(err)
	}
	manager := NewManager(store, eng, lib, NewEventBus())
	mux := http.NewServeMux()
	mux.HandleFunc("/api/knowledge_packs", jsonHandler(func(w http.ResponseWriter, r *http.Request) (any, int, error) {
		limit := queryInt(r, "limit", 80)
		results, err := manager.SearchKnowledgePacksWithOptions(knowledge.SearchOptions{
			Query:   r.URL.Query().Get("q"),
			Domain:  r.URL.Query().Get("domain"),
			Rule:    r.URL.Query().Get("rule"),
			Pattern: r.URL.Query().Get("pattern"),
			Limit:   limit,
		})
		if err != nil {
			return nil, http.StatusBadRequest, err
		}
		return results, http.StatusOK, nil
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/knowledge_packs?domain=code&rule=contract&pattern=api", nil)
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	var body []knowledge.SearchResult
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body) != 1 || body[0].Topic != "code" || len(body[0].RuleMatches) != 1 {
		t.Fatalf("body = %#v", body)
	}
}

func TestSelfEvolvingCandidateApprovalCreatesPermanentPack(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	store, err := database.Open(filepath.Join(root, "loja.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	eng, err := engine.New()
	if err != nil {
		t.Fatal(err)
	}
	lib := knowledge.New(store)
	manager := NewManager(store, eng, lib, NewEventBus())
	contract, err := store.CreateContract("implementar codigo com artefato auditavel", "SQLite como fonte de verdade", "artefatos e auditoria")
	if err != nil {
		t.Fatal(err)
	}
	task, err := store.AddTaskWithPayload(contract.ID, "Implementar codigo auditavel", "Desenvolvedor", nil, map[string]any{
		"domain": "code",
		"key":    "code-implement",
	})
	if err != nil {
		t.Fatal(err)
	}
	payload := map[string]any{
		"artifacts": []fileArtifact{{Path: "projects/contract-1/fixture.json", SHA256: "abc", Bytes: 2}},
	}
	if err := manager.createKnowledgeCandidate(task, payload); err != nil {
		t.Fatal(err)
	}
	pending, err := manager.KnowledgeCandidateSummaries("pending", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 1 || pending[0].Status != "pending" || pending[0].Domain != "code" {
		t.Fatalf("pending = %#v", pending)
	}
	var evidence map[string]any
	if err := json.Unmarshal([]byte(pending[0].Evidence), &evidence); err != nil {
		t.Fatalf("evidence must be structured JSON: %s", pending[0].Evidence)
	}
	if evidence["contract_hash"] != contract.Hash || evidence["task_id"].(float64) != float64(task.ID) {
		t.Fatalf("bad evidence = %#v", evidence)
	}
	if _, err := lib.Load("learned-code"); err == nil {
		t.Fatal("candidate must not become a permanent pack before approval")
	}
	approved, err := manager.ApproveKnowledgeCandidate(pending[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if approved.Status != "approved" {
		t.Fatalf("approved = %#v", approved)
	}
	pack, err := lib.Load("learned-code")
	if err != nil {
		t.Fatal(err)
	}
	if len(pack.PackRules) != 1 || pack.PackRules[0].Action != pending[0].Action {
		t.Fatalf("pack = %#v", pack)
	}
	info := manager.engine.Info()
	found := false
	for _, loaded := range info.Packs {
		if loaded.Domain == "learned-code" && loaded.RuleCount == 1 {
			found = true
		}
	}
	if !found {
		t.Fatalf("learned pack not loaded in engine: %#v", info.Packs)
	}
}

func TestDomainArtifactValidationMatrix(t *testing.T) {
	codeInventory := artifactInventory{Total: 2, JSON: 1, MD: 1}
	if _, err := validateDomainArtifacts("code", codeInventory); err != nil {
		t.Fatal(err)
	}
	documentInventory := artifactInventory{Total: 1, JSON: 1}
	if _, err := validateDomainArtifacts("document", documentInventory); err == nil {
		t.Fatal("document domain should require markdown artifact")
	}
	analysisInventory := artifactInventory{Total: 1, JSON: 1}
	if _, err := validateDomainArtifacts("analysis", analysisInventory); err != nil {
		t.Fatal(err)
	}
}

func runtimeTestMux(manager *Manager, eng *engine.Engine, dbPath, pluginsDir, logPath string) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/health", jsonHandler(func(w http.ResponseWriter, r *http.Request) (any, int, error) {
		health := runtimeHealth(manager, dbPath, pluginsDir, logPath)
		return health, http.StatusOK, nil
	}))
	mux.HandleFunc("/api/engine", jsonHandler(func(w http.ResponseWriter, r *http.Request) (any, int, error) {
		return eng.Info(), http.StatusOK, nil
	}))
	mux.HandleFunc("/api/snapshot", jsonHandler(func(w http.ResponseWriter, r *http.Request) (any, int, error) {
		return manager.Snapshot(), http.StatusOK, nil
	}))
	mux.HandleFunc("/api/factory_series", jsonHandler(func(w http.ResponseWriter, r *http.Request) (any, int, error) {
		var body FactorySeriesRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			return nil, http.StatusBadRequest, err
		}
		result, err := manager.CreateFactorySeries(body)
		if err != nil {
			return nil, http.StatusBadRequest, err
		}
		return result, http.StatusCreated, nil
	}))
	mux.HandleFunc("/api/factory_batches/action", jsonHandler(func(w http.ResponseWriter, r *http.Request) (any, int, error) {
		var body FactoryBatchActionRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			return nil, http.StatusBadRequest, err
		}
		result, err := manager.FactoryBatchAction(body)
		if err != nil {
			return nil, http.StatusBadRequest, err
		}
		return result, http.StatusOK, nil
	}))
	mux.HandleFunc("/api/knowledge_candidates", jsonHandler(func(w http.ResponseWriter, r *http.Request) (any, int, error) {
		var body struct {
			ID     int64  `json:"id"`
			Action string `json:"action"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			return nil, http.StatusBadRequest, err
		}
		switch body.Action {
		case "approve":
			item, err := manager.ApproveKnowledgeCandidate(body.ID)
			return item, http.StatusOK, err
		case "reject":
			item, err := manager.RejectKnowledgeCandidate(body.ID)
			return item, http.StatusOK, err
		default:
			return nil, http.StatusBadRequest, fmt.Errorf("acao de regra candidata desconhecida")
		}
	}))
	return mux
}

func TestRotateRuntimeLog(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "omni-bot-go.log")
	if err := os.WriteFile(path, []byte("1234567890"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := rotateRuntimeLog(path, 5, 3); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("log atual deveria ter sido rotacionado, err=%v", err)
	}
	if raw, err := os.ReadFile(path + ".1"); err != nil || string(raw) != "1234567890" {
		t.Fatalf("rotacionado = %q err=%v", string(raw), err)
	}
}
