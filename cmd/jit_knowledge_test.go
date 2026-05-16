package main

import (
	"path/filepath"
	"testing"

	"omni-bot-go/database"
	"omni-bot-go/engine"
	"omni-bot-go/knowledge"
)

func TestJITKnowledgeFeedsTaskExecution(t *testing.T) {
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
	if _, err := lib.IngestText("sqlite modernc", "fixture", `
- Deve usar modernc sqlite sem CGO.
- Sempre preservar contratos selados em SQLite.
`); err != nil {
		t.Fatal(err)
	}

	contract, err := store.CreateContract(
		"Construir modulo SQLite modernc",
		"Usar SQLite modernc sem CGO e preservar contrato selado.",
		"Artefato auditavel com relatorio JSON.",
	)
	if err != nil {
		t.Fatal(err)
	}
	task, err := store.AddTask(contract.ID, "Registrar contrato SQLite modernc", "Gerente", nil)
	if err != nil {
		t.Fatal(err)
	}

	payload, err := manager.execute(task)
	if err != nil {
		t.Fatal(err)
	}
	items, ok := payload["jit_knowledge_packs"].([]jitKnowledgePackItem)
	if !ok {
		t.Fatalf("jit_knowledge_packs ausente: %#v", payload["jit_knowledge_packs"])
	}
	if len(items) != 1 || items[0].Topic != "sqlite modernc" {
		t.Fatalf("jit_knowledge_packs = %#v", items)
	}
	if items[0].Rules["regra_01"] == "" {
		t.Fatalf("rules = %#v", items[0].Rules)
	}
}
