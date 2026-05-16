package symbolic

import (
	"encoding/json"
	"testing"

	"omni-bot-go/knowledge"
)

var benchmarkContract = json.RawMessage(`{
	"north_star":"implementar codigo Go com seguranca, auditoria e automacao",
	"constraints":"sem modelos locais, com SQLite, sandbox e dashboard",
	"deliverables":"API testada, packs carregados e logs auditaveis"
}`)

func benchmarkEngine() *SymbolicEngine {
	eng := NewSymbolicEngine()
	eng.LoadKnowledgePack(&knowledge.KnowledgePack{
		Domain:  "code",
		Version: "2.0",
		Rules: []knowledge.Rule{
			{Pattern: "Go", Action: "go_contract", Template: "Validar contrato Go com testes."},
			{Pattern: "SQLite", Action: "sqlite_audit", Template: "Persistir trilha auditavel em SQLite."},
			{Pattern: "sandbox", Action: "sandbox_gate", Template: "Executar comandos dentro do sandbox controlado."},
		},
	})
	return eng
}

func BenchmarkPlanHierarchical(b *testing.B) {
	eng := benchmarkEngine()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := eng.PlanHierarchical(benchmarkContract); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkExecuteTask(b *testing.B) {
	eng := benchmarkEngine()
	task := Task{
		ID:          "bench-execute",
		Domain:      "code",
		Role:        "Desenvolvedor",
		Description: "implementar modulo Go com SQLite e sandbox",
		Payload:     benchmarkContract,
		Status:      StatusPending,
	}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := eng.ExecuteTask(task); err != nil {
			b.Fatal(err)
		}
	}
}
