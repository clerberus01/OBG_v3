package knowledge

import (
	"os"
	"path/filepath"
	"testing"

	"omni-bot-go/database"
)

func TestIngestSearchAndSummaries(t *testing.T) {
	store, err := database.Open(filepath.Join(t.TempDir(), "loja.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	lib := New(store)
	item, err := lib.IngestText("protocolo jit", "teste", `
- Deve registrar origem local.
- Evitar lero-lero e respostas sem regra auditavel.
- Sempre preservar contexto aprovado.
Texto de apoio sobre gofmt, contratos selados e recuperacao de contexto.
`)
	if err != nil {
		t.Fatal(err)
	}
	if item.Topic != "protocolo jit" {
		t.Fatalf("topic = %q", item.Topic)
	}
	if len(item.Rules) != 3 {
		t.Fatalf("rules = %d", len(item.Rules))
	}

	loaded, err := lib.Load("protocolo jit")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Source != "teste" || loaded.Summary == "" {
		t.Fatalf("loaded = %+v", loaded)
	}

	results, err := lib.Search("gofmt contexto aprovado", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Topic != "protocolo jit" {
		t.Fatalf("results = %+v", results)
	}

	summaries, err := lib.Summaries(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(summaries) != 1 || summaries[0].RuleCount != 3 {
		t.Fatalf("summaries = %+v", summaries)
	}
}

func TestInitialPackFilesLoad(t *testing.T) {
	files := []string{
		"core.pack.json",
		"code.pack.json",
		"docs.pack.json",
		"audit.pack.json",
		"automation.pack.json",
		"web-services.pack.json",
		"local-tools.pack.json",
	}
	for _, file := range files {
		t.Run(file, func(t *testing.T) {
			path := filepath.Join(".", file)
			if _, err := os.Stat(path); err != nil {
				t.Fatalf("pack file missing: %v", err)
			}
			pack, err := LoadFromFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if pack.Domain == "" || pack.Version == "" {
				t.Fatalf("invalid metadata: %#v", pack)
			}
			if len(pack.Rules) == 0 {
				t.Fatalf("pack without rules: %#v", pack)
			}
			for i, rule := range pack.Rules {
				if rule.Action == "" || rule.Template == "" {
					t.Fatalf("rule %d incomplete: %#v", i, rule)
				}
			}
		})
	}
}

func TestSearchByDomainRuleAndPattern(t *testing.T) {
	store, err := database.Open(filepath.Join(t.TempDir(), "loja.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	lib := New(store)
	if err := lib.SaveKnowledgePack(&KnowledgePack{
		Domain:  "code",
		Version: "2.0",
		Rules: []Rule{
			{Pattern: "api", Action: "contract_api", Template: "Validar contrato de API e payload JSON."},
			{Pattern: "build", Action: "build_gate", Template: "Executar build antes de selar."},
		},
	}, "test"); err != nil {
		t.Fatal(err)
	}
	if err := lib.SaveKnowledgePack(&KnowledgePack{
		Domain:  "audit",
		Version: "2.0",
		Rules: []Rule{
			{Pattern: "read-only", Action: "readonly_guard", Template: "Nao reescrever artefato aprovado."},
		},
	}, "test"); err != nil {
		t.Fatal(err)
	}

	byDomain, err := lib.SearchWithOptions(SearchOptions{Domain: "code", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(byDomain) != 1 || byDomain[0].Topic != "code" {
		t.Fatalf("by domain = %#v", byDomain)
	}

	byRule, err := lib.SearchWithOptions(SearchOptions{Rule: "readonly", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(byRule) != 1 || byRule[0].Topic != "audit" || len(byRule[0].RuleMatches) != 1 {
		t.Fatalf("by rule = %#v", byRule)
	}

	byPattern, err := lib.SearchWithOptions(SearchOptions{Pattern: "api", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(byPattern) != 1 || byPattern[0].Topic != "code" || byPattern[0].RuleMatches[0].Pattern != "api" {
		t.Fatalf("by pattern = %#v", byPattern)
	}

	combined, err := lib.SearchWithOptions(SearchOptions{Domain: "code", Rule: "contract", Pattern: "api", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(combined) != 1 || combined[0].Topic != "code" || combined[0].RuleMatches[0].Action != "contract_api" {
		t.Fatalf("combined = %#v", combined)
	}
}
