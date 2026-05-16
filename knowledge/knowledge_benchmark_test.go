package knowledge

import (
	"path/filepath"
	"testing"

	"omni-bot-go/database"
)

func BenchmarkLoadPackFromFile(b *testing.B) {
	path := filepath.Join(".", "core.pack.json")
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := LoadFromFile(path); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkSearchWithOptions(b *testing.B) {
	store, err := database.Open(filepath.Join(b.TempDir(), "bench.db"))
	if err != nil {
		b.Fatal(err)
	}
	defer store.Close()

	lib := New(store)
	packs := []*KnowledgePack{
		{Domain: "code", Version: "2.0", Rules: []Rule{
			{Pattern: "api", Action: "contract_api", Template: "Validar contrato de API e payload JSON."},
			{Pattern: "build", Action: "build_gate", Template: "Executar build antes de selar."},
		}},
		{Domain: "audit", Version: "2.0", Rules: []Rule{
			{Pattern: "read-only", Action: "readonly_guard", Template: "Nao reescrever artefato aprovado."},
			{Pattern: "hash", Action: "immutable_hash", Template: "Preservar hash imutavel no SQLite."},
		}},
	}
	for _, pack := range packs {
		if err := lib.SaveKnowledgePack(pack, "benchmark"); err != nil {
			b.Fatal(err)
		}
	}

	options := SearchOptions{Domain: "code", Rule: "contract", Pattern: "api", Limit: 10}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := lib.SearchWithOptions(options); err != nil {
			b.Fatal(err)
		}
	}
}
