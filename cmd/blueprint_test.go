package main

import (
	"path/filepath"
	"testing"

	"omni-bot-go/database"
	"omni-bot-go/engine"
	"omni-bot-go/knowledge"
)

const blueprintFixture = `
north_star: "Construir modulo operacional por YAML"
constraints:
  - "SQLite modernc e fonte de verdade."
  - "Tarefas aprovadas sao read-only."
deliverables:
  - "Contrato selado."
  - "Checklist executavel."
tasks:
  - id: contrato
    title: "Registrar contrato importado"
    role: "Gerente"
    payload:
      command: "write_contract"
      validation: "hash"
  - id: plano
    title: "Planejar execucao por modulos"
    role: "Arquiteto"
    depends_on: [contrato]
    payload:
      command: "plan_modules"
  - id: auditoria
    title: "Auditar saida YAML"
    role: "Auditor Tecnico"
    depends_on: [contrato, plano]
`

func TestParseCommandBlueprintYAML(t *testing.T) {
	blueprint, err := ParseCommandBlueprintYAML(blueprintFixture)
	if err != nil {
		t.Fatal(err)
	}
	if blueprint.NorthStar != "Construir modulo operacional por YAML" {
		t.Fatalf("north star = %q", blueprint.NorthStar)
	}
	if len(blueprint.Tasks) != 3 {
		t.Fatalf("tasks = %d", len(blueprint.Tasks))
	}
	if blueprint.Tasks[1].DependsOn[0] != "contrato" {
		t.Fatalf("depends_on = %#v", blueprint.Tasks[1].DependsOn)
	}
	if blueprint.Tasks[0].Payload["command"] != "write_contract" {
		t.Fatalf("payload = %#v", blueprint.Tasks[0].Payload)
	}
}

func TestCreateProjectFromYAML(t *testing.T) {
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
	contract, err := manager.CreateProjectFromYAML(blueprintFixture)
	if err != nil {
		t.Fatal(err)
	}
	if contract.ID == 0 {
		t.Fatal("contract id vazio")
	}
	tasks, err := store.ListTasks()
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 3 {
		t.Fatalf("tasks = %d", len(tasks))
	}
	if tasks[1].Dependencies[0] != tasks[0].ID {
		t.Fatalf("dependency ids = %#v want first task %d", tasks[1].Dependencies, tasks[0].ID)
	}
	if tasks[0].Payload["source"] != "yaml" || tasks[0].Payload["command"] != "write_contract" {
		t.Fatalf("payload = %#v", tasks[0].Payload)
	}
}

func TestHandoffAreaFeedsDependentTask(t *testing.T) {
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
	contract, err := manager.CreateProjectFromYAML(blueprintFixture)
	if err != nil {
		t.Fatal(err)
	}
	tasks, err := store.ListTasks()
	if err != nil {
		t.Fatal(err)
	}
	firstPayload := map[string]any{
		"task_id":     tasks[0].ID,
		"role":        tasks[0].Role,
		"report":      "handoff inicial aprovado",
		"engine_hash": "fixture",
		"artifacts":   []fileArtifact{{Path: "fixture.md", SHA256: "abc", Bytes: 3}},
	}
	if _, err := store.AddHandoff(contract.ID, tasks[0].ID, 0, "handoff", firstPayload); err != nil {
		t.Fatal(err)
	}
	handoffs, err := store.HandoffsForTask(tasks[1])
	if err != nil {
		t.Fatal(err)
	}
	if len(handoffs) != 1 {
		t.Fatalf("handoffs = %d", len(handoffs))
	}
	if handoffs[0].FromTaskID != tasks[0].ID || handoffs[0].Payload["report"] != "handoff inicial aprovado" {
		t.Fatalf("handoff = %#v", handoffs[0])
	}
	previous, err := manager.previousReports(tasks[1])
	if err != nil {
		t.Fatal(err)
	}
	if len(previous) == 0 || previous[0]["source"] != "handoff_area" {
		t.Fatalf("previous reports = %#v", previous)
	}
}

func TestCreateHandoffsTargetsDependentTasks(t *testing.T) {
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
	contract, err := manager.CreateProjectFromYAML(blueprintFixture)
	if err != nil {
		t.Fatal(err)
	}
	tasks, err := store.ListTasks()
	if err != nil {
		t.Fatal(err)
	}
	payload := map[string]any{
		"task_id": tasks[0].ID,
		"report":  "saida aprovada",
	}
	if err := manager.createHandoffs(tasks[0], payload); err != nil {
		t.Fatal(err)
	}
	handoffs, err := store.ListHandoffs(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(handoffs) != 2 {
		t.Fatalf("handoffs = %#v", handoffs)
	}
	destinations := map[int64]bool{}
	for _, handoff := range handoffs {
		if handoff.FromTaskID != tasks[0].ID || handoff.ToTaskID == 0 {
			t.Fatalf("handoff should be directed from first task: %#v", handoff)
		}
		destinations[handoff.ToTaskID] = true
		meta, ok := handoff.Payload["handoff"].(map[string]any)
		if !ok || meta["from_task_id"] == nil || meta["to_task_id"] == nil {
			t.Fatalf("handoff metadata missing: %#v", handoff.Payload)
		}
	}
	if !destinations[tasks[1].ID] || !destinations[tasks[2].ID] {
		t.Fatalf("destinations = %#v contract=%d", destinations, contract.ID)
	}
	targeted, err := store.HandoffsForTask(tasks[1])
	if err != nil {
		t.Fatal(err)
	}
	if len(targeted) != 1 || targeted[0].ToTaskID != tasks[1].ID {
		t.Fatalf("targeted = %#v", targeted)
	}
}
