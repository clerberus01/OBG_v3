package symbolic

import (
	"encoding/json"
	"strings"
	"testing"

	"omni-bot-go/knowledge"
)

func TestSymbolicEngineExecutesMatchingRules(t *testing.T) {
	eng := NewSymbolicEngine()
	eng.LoadKnowledgePack(&knowledge.KnowledgePack{
		Domain:  "teste",
		Version: "1.0",
		Rules: []knowledge.Rule{
			{Pattern: "sqlite", Action: "solid_state", Template: "Usar SQLite para {{.Description}}."},
		},
	})

	raw, err := eng.ExecuteTask(Task{ID: "t1", Role: "Gerente", Description: "validar contrato sqlite", Status: StatusPending})
	if err != nil {
		t.Fatal(err)
	}
	var result ExecutionResult
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatal(err)
	}
	if len(result.RulesApplied) != 1 || !strings.Contains(result.RulesApplied[0].Rendered, "Usar SQLite") {
		t.Fatalf("rules = %#v", result.RulesApplied)
	}
	if !strings.Contains(result.Report, "Symbolic Engine") || result.Status != StatusDone || !result.Deterministic {
		t.Fatalf("result = %#v", result)
	}
	if eng.Info().RuleCount != 1 {
		t.Fatalf("info = %#v", eng.Info())
	}
}

func TestSymbolicEngineUsesDeterministicRoleTemplates(t *testing.T) {
	eng := NewSymbolicEngine()
	cases := []struct {
		role string
		want string
	}{
		{role: "Gerente", want: "Confirmar objetivo e limites do contrato"},
		{role: "Arquiteto", want: "Decompor a entrega em modulos pequenos"},
		{role: "Desenvolvedor", want: "Implementar somente o escopo contratado"},
		{role: "Analista", want: "Definir perguntas, fontes e criterios"},
		{role: "Redator", want: "Estruturar mensagem, publico e objetivo"},
		{role: "Auditor Tecnico", want: "Verificar contrato, hash, artefatos e handoffs"},
	}
	for _, tc := range cases {
		t.Run(tc.role, func(t *testing.T) {
			raw, err := eng.ExecuteTask(Task{
				ID:          "role-template",
				Domain:      "test",
				Role:        tc.role,
				Description: "executar contrato de teste",
				Status:      StatusPending,
			})
			if err != nil {
				t.Fatal(err)
			}
			var result ExecutionResult
			if err := json.Unmarshal(raw, &result); err != nil {
				t.Fatal(err)
			}
			if !containsLine(result.RoleTemplate, tc.want) || !strings.Contains(result.Report, "Template do Papel:") {
				t.Fatalf("role %s result missing template %q:\n%#v", tc.role, tc.want, result)
			}
		})
	}
}

func TestSymbolicEngineExecuteTaskReturnsStructuredJSON(t *testing.T) {
	eng := NewSymbolicEngine()
	raw, err := eng.ExecuteTask(Task{
		ID:          "structured",
		Domain:      "code",
		Role:        "Desenvolvedor",
		Description: "implementar contrato sqlite",
		Payload:     json.RawMessage(`{"contract_hash":"abc"}`),
		Status:      StatusPending,
	})
	if err != nil {
		t.Fatal(err)
	}
	var result ExecutionResult
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatal(err)
	}
	if result.TaskID != "structured" || result.Domain != "code" || result.Role != "Desenvolvedor" {
		t.Fatalf("bad identity: %#v", result)
	}
	if result.Report == "" || len(result.RoleTemplate) == 0 || len(result.Evidence) == 0 || len(result.NextActions) == 0 {
		t.Fatalf("incomplete structured result: %#v", result)
	}
	if !json.Valid(result.InputPayload) {
		t.Fatalf("input payload should remain valid JSON: %s", result.InputPayload)
	}
}

func containsLine(lines []string, fragment string) bool {
	for _, line := range lines {
		if strings.Contains(line, fragment) {
			return true
		}
	}
	return false
}

type domainCase struct {
	name     string
	contract string
	domain   string
	ids      []string
	roles    []string
}

func symbolicDomainCases() []domainCase {
	return []domainCase{
		{
			name:     "code",
			contract: `{"north_star":"implementar codigo Go para API com testes"}`,
			domain:   "code",
			ids:      []string{"contract", "code-scope", "code-implement", "code-verify", "audit"},
			roles:    []string{"Gerente", "Arquiteto", "Desenvolvedor", "Auditor Tecnico", "Auditor Tecnico"},
		},
		{
			name:     "document",
			contract: `{"north_star":"criar documento tecnico e manual de uso"}`,
			domain:   "document",
			ids:      []string{"contract", "document-brief", "document-draft", "document-review", "audit"},
			roles:    []string{"Gerente", "Redator Tecnico", "Redator", "Auditor Tecnico", "Auditor Tecnico"},
		},
		{
			name:     "analysis",
			contract: `{"north_star":"fazer analise de dados e metricas de risco"}`,
			domain:   "analysis",
			ids:      []string{"contract", "analysis-frame", "analysis-execute", "analysis-validate", "audit"},
			roles:    []string{"Gerente", "Analista", "Analista de Dados", "Auditor Tecnico", "Auditor Tecnico"},
		},
		{
			name:     "automation",
			contract: `{"north_star":"automatizar processo com script e pipeline"}`,
			domain:   "automation",
			ids:      []string{"contract", "automation-map", "automation-build", "automation-test", "audit"},
			roles:    []string{"Gerente", "Arquiteto de Automacao", "Operario Polimorfico", "Auditor Tecnico", "Auditor Tecnico"},
		},
		{
			name:     "design",
			contract: `{"north_star":"desenhar interface dashboard com layout visual"}`,
			domain:   "design",
			ids:      []string{"contract", "design-brief", "design-produce", "design-audit", "audit"},
			roles:    []string{"Gerente", "Designer", "Designer de Produto", "Auditor Tecnico", "Auditor Tecnico"},
		},
		{
			name:     "strategy",
			contract: `{"north_star":"definir estrategia de negocio e roadmap de mercado"}`,
			domain:   "strategy",
			ids:      []string{"contract", "strategy-context", "strategy-plan", "strategy-review", "audit"},
			roles:    []string{"Gerente", "Estrategista", "Analista Estrategico", "Auditor Tecnico", "Auditor Tecnico"},
		},
	}
}

func TestSymbolicPlannerRequiresContract(t *testing.T) {
	eng := NewSymbolicEngine()
	if _, err := eng.PlanHierarchical(json.RawMessage(`{}`)); err == nil {
		t.Fatal("expected missing north_star error")
	}
	tasks, err := eng.PlanHierarchical(json.RawMessage(`{"north_star":"entregar relatorio"}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 5 || tasks[0].ID != "contract" || tasks[4].Dependencies[0] != "document-review" {
		t.Fatalf("tasks = %#v", tasks)
	}
}

func TestSymbolicPlannerGeneratesDomainSpecificTasks(t *testing.T) {
	eng := NewSymbolicEngine()
	for _, tc := range symbolicDomainCases() {
		t.Run(tc.name, func(t *testing.T) {
			tasks, err := eng.PlanHierarchical(json.RawMessage(tc.contract))
			if err != nil {
				t.Fatal(err)
			}
			if len(tasks) != len(tc.ids) {
				t.Fatalf("tasks = %d %#v", len(tasks), tasks)
			}
			if len(tasks[0].Dependencies) != 0 {
				t.Fatalf("contract task must not depend on itself: %#v", tasks[0])
			}
			for i, task := range tasks {
				if task.ID != tc.ids[i] {
					t.Fatalf("task[%d] id=%s want %s; tasks=%#v", i, task.ID, tc.ids[i], tasks)
				}
				if task.Role != tc.roles[i] {
					t.Fatalf("task[%d] role=%s want %s; tasks=%#v", i, task.Role, tc.roles[i], tasks)
				}
				if task.Domain != tc.domain {
					t.Fatalf("task %s domain=%s want %s; tasks=%#v", task.ID, task.Domain, tc.domain, tasks)
				}
			}
			for i := 1; i < len(tasks); i++ {
				if len(tasks[i].Dependencies) != 1 || tasks[i].Dependencies[0] != tasks[i-1].ID {
					t.Fatalf("task[%d] should depend on previous task: %#v", i, tasks[i])
				}
			}
		})
	}
}

func TestSymbolicEngineExecutesPlannedDomainTasks(t *testing.T) {
	eng := NewSymbolicEngine()
	for _, tc := range symbolicDomainCases() {
		t.Run(tc.name, func(t *testing.T) {
			tasks, err := eng.PlanHierarchical(json.RawMessage(tc.contract))
			if err != nil {
				t.Fatal(err)
			}
			for _, task := range tasks {
				raw, err := eng.ExecuteTask(task)
				if err != nil {
					t.Fatalf("execute %s: %v", task.ID, err)
				}
				var result ExecutionResult
				if err := json.Unmarshal(raw, &result); err != nil {
					t.Fatalf("decode %s: %v", task.ID, err)
				}
				if result.TaskID != task.ID || result.Domain != tc.domain || result.Role != task.Role {
					t.Fatalf("bad execution identity for %s: %#v", task.ID, result)
				}
				if result.Instruction != task.Description || result.Status != StatusDone || !result.Deterministic {
					t.Fatalf("bad execution state for %s: %#v", task.ID, result)
				}
				if len(result.RoleTemplate) == 0 || len(result.Evidence) == 0 || len(result.NextActions) == 0 {
					t.Fatalf("incomplete execution for %s: %#v", task.ID, result)
				}
				if !containsLine(result.Evidence, "domain:"+tc.domain) {
					t.Fatalf("domain evidence missing for %s: %#v", task.ID, result.Evidence)
				}
				if !json.Valid(result.InputPayload) {
					t.Fatalf("invalid payload for %s: %s", task.ID, result.InputPayload)
				}
			}
		})
	}
}

func TestSymbolicPlannerUsesMatchingKnowledgePackRules(t *testing.T) {
	eng := NewSymbolicEngine()
	eng.LoadKnowledgePack(&knowledge.KnowledgePack{
		Domain:  "security",
		Version: "1.0",
		Rules: []knowledge.Rule{
			{
				Pattern:  "seguranca",
				Action:   "security_review",
				Template: "Validar permissoes, logs e reversibilidade antes de selar.",
			},
		},
	})
	tasks, err := eng.PlanHierarchical(json.RawMessage(`{
		"north_star":"implementar codigo Go com seguranca e auditoria",
		"constraints":"registrar logs tecnicos",
		"deliverables":"API testada"
	}`))
	if err != nil {
		t.Fatal(err)
	}
	var ruleTask *Task
	for i := range tasks {
		if tasks[i].ID == "rule-security-security-review" {
			ruleTask = &tasks[i]
			break
		}
	}
	if ruleTask == nil {
		t.Fatalf("rule-derived task not found: %#v", tasks)
	}
	if ruleTask.Role != "Auditor Tecnico" || ruleTask.Dependencies[0] != "contract" {
		t.Fatalf("bad rule task: %#v", ruleTask)
	}
	if tasks[2].Dependencies[0] != ruleTask.ID {
		t.Fatalf("domain task should depend on last rule task: %#v", tasks)
	}
	var payload struct {
		Domain       string            `json:"domain"`
		MatchedRules []Rule            `json:"matched_rules"`
		Contract     map[string]string `json:"contract"`
	}
	if err := json.Unmarshal(tasks[2].Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Domain != "code" || len(payload.MatchedRules) != 1 || payload.Contract["north_star"] == "" {
		t.Fatalf("payload = %#v", payload)
	}
}
