package symbolic

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"text/template"
	"time"

	"omni-bot-go/knowledge"
)

const (
	StatusPending = "pending"
	StatusRunning = "running"
	StatusDone    = "done"
	StatusFailed  = "failed"
)

type Task struct {
	ID           string          `json:"id"`
	Domain       string          `json:"domain"`
	Description  string          `json:"description"`
	Role         string          `json:"role"`
	Dependencies []string        `json:"dependencies"`
	Payload      json.RawMessage `json:"payload,omitempty"`
	Status       string          `json:"status"`
}

type Rule struct {
	Pattern  string `json:"pattern"`
	Action   string `json:"action"`
	Template string `json:"template"`
	Domain   string `json:"domain"`
}

type PackInfo struct {
	Domain    string    `json:"domain"`
	Version   string    `json:"version"`
	RuleCount int       `json:"rule_count"`
	LoadedAt  time.Time `json:"loaded_at"`
	ExpiresAt time.Time `json:"expires_at,omitempty"`
}

type Info struct {
	RuleCount int        `json:"rule_count"`
	Packs     []PackInfo `json:"packs"`
}

type ExecutionResult struct {
	TaskID        string          `json:"task_id"`
	Domain        string          `json:"domain,omitempty"`
	Role          string          `json:"role"`
	Instruction   string          `json:"instruction"`
	Status        string          `json:"status"`
	Summary       string          `json:"summary"`
	InputPayload  json.RawMessage `json:"input_payload,omitempty"`
	RoleTemplate  []string        `json:"role_template"`
	RulesApplied  []AppliedRule   `json:"rules_applied"`
	Evidence      []string        `json:"evidence"`
	NextActions   []string        `json:"next_actions"`
	Report        string          `json:"report"`
	GeneratedBy   string          `json:"generated_by"`
	Deterministic bool            `json:"deterministic"`
}

type AppliedRule struct {
	Domain   string `json:"domain"`
	Pattern  string `json:"pattern,omitempty"`
	Action   string `json:"action,omitempty"`
	Rendered string `json:"rendered"`
}

type SymbolicEngine struct {
	mu             sync.RWMutex
	knowledgePacks map[string]*knowledge.KnowledgePack
	rules          map[string][]Rule
}

func NewSymbolicEngine() *SymbolicEngine {
	return &SymbolicEngine{
		knowledgePacks: map[string]*knowledge.KnowledgePack{},
		rules:          map[string][]Rule{},
	}
}

func (e *SymbolicEngine) LoadKnowledgePack(pack *knowledge.KnowledgePack) {
	if pack == nil || strings.TrimSpace(pack.Domain) == "" {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	copyPack := *pack
	if copyPack.LoadedAt.IsZero() {
		copyPack.LoadedAt = time.Now()
	}
	e.knowledgePacks[copyPack.Domain] = &copyPack
	rules := make([]Rule, 0, len(copyPack.Rules))
	for _, rule := range copyPack.Rules {
		rules = append(rules, Rule{
			Pattern:  rule.Pattern,
			Action:   rule.Action,
			Template: rule.Template,
			Domain:   copyPack.Domain,
		})
	}
	e.rules[copyPack.Domain] = rules
}

func (e *SymbolicEngine) Info() Info {
	e.mu.RLock()
	defer e.mu.RUnlock()
	var info Info
	for domain, pack := range e.knowledgePacks {
		ruleCount := len(e.rules[domain])
		info.RuleCount += ruleCount
		info.Packs = append(info.Packs, PackInfo{
			Domain:    domain,
			Version:   pack.Version,
			RuleCount: ruleCount,
			LoadedAt:  pack.LoadedAt,
			ExpiresAt: pack.ExpiresAt,
		})
	}
	sort.Slice(info.Packs, func(i, j int) bool {
		return info.Packs[i].Domain < info.Packs[j].Domain
	})
	return info
}

func (e *SymbolicEngine) Hash() string {
	info := e.Info()
	raw, _ := json.Marshal(info)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func (e *SymbolicEngine) PlanHierarchical(contractJSON json.RawMessage) ([]Task, error) {
	var contract struct {
		NorthStar    string `json:"north_star"`
		Constraints  string `json:"constraints"`
		Deliverables string `json:"deliverables"`
	}
	_ = json.Unmarshal(contractJSON, &contract)
	if strings.TrimSpace(contract.NorthStar) == "" {
		return nil, fmt.Errorf("contrato sem north_star")
	}
	contractText := contract.NorthStar + " " + contract.Constraints + " " + contract.Deliverables
	e.mu.RLock()
	matchedRules := e.matchingRulesForTextLocked(contractText)
	e.mu.RUnlock()
	domain := detectDomainWithRules(contractText, matchedRules)
	return buildPlan(domain, contract, matchedRules), nil
}

type domainDefinition struct {
	name     string
	keywords []string
	steps    []Task
}

var domainDefinitions = []domainDefinition{
	{
		name:     "code",
		keywords: []string{"codigo", "programa", "software", "api", "backend", "frontend", "go", "javascript", "typescript", "bug", "teste", "sistema"},
		steps: []Task{
			{ID: "code-scope", Description: "Mapear requisitos tecnicos, arquivos afetados e crivos de build/teste", Role: "Arquiteto"},
			{ID: "code-implement", Description: "Implementar alteracoes de codigo com artefatos versionaveis", Role: "Desenvolvedor", Dependencies: []string{"code-scope"}},
			{ID: "code-verify", Description: "Executar validacoes tecnicas, revisar diffs e registrar evidencias", Role: "Auditor Tecnico", Dependencies: []string{"code-implement"}},
		},
	},
	{
		name:     "document",
		keywords: []string{"documento", "relatorio", "manual", "artigo", "texto", "redacao", "proposta", "especificacao", "conteudo"},
		steps: []Task{
			{ID: "document-brief", Description: "Definir publico, objetivo, escopo e estrutura do documento", Role: "Redator Tecnico"},
			{ID: "document-draft", Description: "Produzir versao estruturada com secoes e evidencias", Role: "Redator", Dependencies: []string{"document-brief"}},
			{ID: "document-review", Description: "Revisar clareza, consistencia, completude e aderencia ao contrato", Role: "Auditor Tecnico", Dependencies: []string{"document-draft"}},
		},
	},
	{
		name:     "analysis",
		keywords: []string{"analise", "dados", "metricas", "diagnostico", "indicadores", "estatistica", "comparacao", "avaliacao", "risco"},
		steps: []Task{
			{ID: "analysis-frame", Description: "Definir perguntas analiticas, fontes, criterios e limites", Role: "Analista"},
			{ID: "analysis-execute", Description: "Processar evidencias e produzir achados auditaveis", Role: "Analista de Dados", Dependencies: []string{"analysis-frame"}},
			{ID: "analysis-validate", Description: "Validar consistencia, lacunas, vieses e rastreabilidade dos achados", Role: "Auditor Tecnico", Dependencies: []string{"analysis-execute"}},
		},
	},
	{
		name:     "automation",
		keywords: []string{"automacao", "automatizar", "script", "pipeline", "processo", "integracao", "webhook", "agendamento", "rotina"},
		steps: []Task{
			{ID: "automation-map", Description: "Mapear gatilhos, entradas, saidas, permissoes e falhas esperadas", Role: "Arquiteto de Automacao"},
			{ID: "automation-build", Description: "Construir rotina automatizada com controles e logs", Role: "Operario Polimorfico", Dependencies: []string{"automation-map"}},
			{ID: "automation-test", Description: "Testar caminho feliz, falhas, idempotencia e reversibilidade", Role: "Auditor Tecnico", Dependencies: []string{"automation-build"}},
		},
	},
	{
		name:     "design",
		keywords: []string{"design", "interface", "layout", "dashboard", "visual", "marca", "prototipo", "ux", "ui", "tela"},
		steps: []Task{
			{ID: "design-brief", Description: "Definir usuario, contexto, hierarquia visual e restricoes de interface", Role: "Designer"},
			{ID: "design-produce", Description: "Produzir solucao visual ou especificacao de interface", Role: "Designer de Produto", Dependencies: []string{"design-brief"}},
			{ID: "design-audit", Description: "Auditar acessibilidade, responsividade, clareza e aderencia ao contrato", Role: "Auditor Tecnico", Dependencies: []string{"design-produce"}},
		},
	},
	{
		name:     "strategy",
		keywords: []string{"estrategia", "plano", "negocio", "roadmap", "mercado", "campanha", "posicionamento", "prioridade", "crescimento"},
		steps: []Task{
			{ID: "strategy-context", Description: "Mapear objetivo, publico, restricoes, riscos e criterios de decisao", Role: "Estrategista"},
			{ID: "strategy-plan", Description: "Construir plano de acao com prioridades, marcos e tradeoffs", Role: "Analista Estrategico", Dependencies: []string{"strategy-context"}},
			{ID: "strategy-review", Description: "Validar coerencia, riscos, dependencias e indicadores de sucesso", Role: "Auditor Tecnico", Dependencies: []string{"strategy-plan"}},
		},
	},
}

func detectDomain(text string) string {
	return detectDomainWithRules(text, nil)
}

func detectDomainWithRules(text string, rules []Rule) string {
	normalized := normalizeDomainText(text)
	bestDomain := "general"
	bestScore := 0
	for _, domain := range domainDefinitions {
		score := 0
		for _, keyword := range domain.keywords {
			if strings.Contains(normalized, normalizeDomainText(keyword)) {
				score++
			}
		}
		if score > bestScore {
			bestScore = score
			bestDomain = domain.name
		}
	}
	for _, rule := range rules {
		ruleDomain := normalizeDomainText(rule.Domain)
		for _, domain := range domainDefinitions {
			if ruleDomain == domain.name {
				if bestScore == 0 {
					bestDomain = domain.name
				}
				if domain.name == bestDomain {
					bestScore++
				}
			}
		}
	}
	return bestDomain
}

func domainPlan(domain string) []Task {
	return buildPlan(domain, struct {
		NorthStar    string `json:"north_star"`
		Constraints  string `json:"constraints"`
		Deliverables string `json:"deliverables"`
	}{}, nil)
}

func buildPlan(domain string, contract struct {
	NorthStar    string `json:"north_star"`
	Constraints  string `json:"constraints"`
	Deliverables string `json:"deliverables"`
}, rules []Rule) []Task {
	tasks := []Task{
		withPlanPayload(Task{ID: "contract", Domain: domain, Description: contractAwareDescription("Validar contrato universal, lacunas e criterios de aceite", contract.NorthStar), Role: "Gerente", Status: StatusPending}, domain, contract, rules),
	}
	ruleTasks := rulePlanTasks(domain, contract, rules)
	tasks = append(tasks, ruleTasks...)
	for _, definition := range domainDefinitions {
		if definition.name != domain {
			continue
		}
		for _, step := range definition.steps {
			task := withPlanPayload(step, domain, contract, rules)
			if len(ruleTasks) > 0 && len(task.Dependencies) == 1 && task.Dependencies[0] == "contract" {
				task.Dependencies = []string{ruleTasks[len(ruleTasks)-1].ID}
			}
			tasks = append(tasks, task)
		}
		return append(tasks, withPlanPayload(Task{ID: "audit", Domain: domain, Description: "Aplicar crivo tecnico final e selar resultado", Role: "Auditor Tecnico", Dependencies: []string{definition.steps[len(definition.steps)-1].ID}, Status: StatusPending}, domain, contract, rules))
	}
	return []Task{
		withPlanPayload(Task{ID: "contract", Domain: "general", Description: contractAwareDescription("Validar contrato universal, lacunas e criterios de aceite", contract.NorthStar), Role: "Gerente", Status: StatusPending}, "general", contract, rules),
		withPlanPayload(Task{ID: "plan", Domain: "general", Description: "Planejar subtarefas e dependencias auditaveis", Role: "Arquiteto", Dependencies: []string{"contract"}, Status: StatusPending}, "general", contract, rules),
		withPlanPayload(Task{ID: "execute", Domain: "general", Description: "Produzir artefatos conforme contrato", Role: "Operario Polimorfico", Dependencies: []string{"plan"}, Status: StatusPending}, "general", contract, rules),
		withPlanPayload(Task{ID: "audit", Domain: "general", Description: "Aplicar crivo tecnico final e selar resultado", Role: "Auditor Tecnico", Dependencies: []string{"execute"}, Status: StatusPending}, "general", contract, rules),
	}
}

func withPlanPayload(task Task, domain string, contract struct {
	NorthStar    string `json:"north_star"`
	Constraints  string `json:"constraints"`
	Deliverables string `json:"deliverables"`
}, rules []Rule) Task {
	task.Domain = domain
	task.Status = StatusPending
	if task.ID != "contract" && len(task.Dependencies) == 0 {
		task.Dependencies = []string{"contract"}
	}
	if len(task.Payload) == 0 {
		task.Payload = taskPayload(domain, contract, rules)
	}
	return task
}

func taskPayload(domain string, contract struct {
	NorthStar    string `json:"north_star"`
	Constraints  string `json:"constraints"`
	Deliverables string `json:"deliverables"`
}, rules []Rule) json.RawMessage {
	payload := map[string]any{
		"domain": domain,
	}
	if strings.TrimSpace(contract.NorthStar) != "" {
		payload["contract"] = map[string]string{
			"north_star":   contract.NorthStar,
			"constraints":  contract.Constraints,
			"deliverables": contract.Deliverables,
		}
	}
	if len(rules) > 0 {
		payload["matched_rules"] = rules
	}
	raw, _ := json.Marshal(payload)
	return raw
}

func rulePlanTasks(domain string, contract struct {
	NorthStar    string `json:"north_star"`
	Constraints  string `json:"constraints"`
	Deliverables string `json:"deliverables"`
}, rules []Rule) []Task {
	seen := map[string]struct{}{}
	var out []Task
	dep := "contract"
	for _, rule := range rules {
		key := ruleTaskID(rule)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		description := ruleTaskDescription(rule)
		task := Task{
			ID:           key,
			Domain:       domain,
			Description:  description,
			Role:         roleForRule(rule),
			Dependencies: []string{dep},
			Status:       StatusPending,
			Payload:      taskPayload(domain, contract, []Rule{rule}),
		}
		out = append(out, task)
		dep = key
	}
	return out
}

func ruleTaskID(rule Rule) string {
	base := strings.TrimSpace(rule.Action)
	if base == "" {
		base = rule.Pattern
	}
	if base == "" {
		base = rule.Domain
	}
	return "rule-" + slug(rule.Domain) + "-" + slug(base)
}

func ruleTaskDescription(rule Rule) string {
	detail := strings.TrimSpace(rule.Template)
	if detail == "" {
		detail = strings.TrimSpace(rule.Action)
	}
	if detail == "" {
		detail = strings.TrimSpace(rule.Pattern)
	}
	return fmt.Sprintf("Aplicar regra %s/%s: %s", rule.Domain, rule.Action, detail)
}

func roleForRule(rule Rule) string {
	action := normalizeDomainText(rule.Action + " " + rule.Template)
	switch {
	case strings.Contains(action, "audit") || strings.Contains(action, "valid") || strings.Contains(action, "crivo") || strings.Contains(action, "review"):
		return "Auditor Tecnico"
	case strings.Contains(action, "document") || strings.Contains(action, "texto") || strings.Contains(action, "redacao"):
		return "Redator"
	case strings.Contains(action, "analise") || strings.Contains(action, "metric"):
		return "Analista"
	case strings.Contains(action, "codigo") || strings.Contains(action, "implementar"):
		return "Desenvolvedor"
	default:
		return "Arquiteto"
	}
}

func contractAwareDescription(base, northStar string) string {
	northStar = strings.TrimSpace(northStar)
	if northStar == "" {
		return base
	}
	return base + ": " + northStar
}

func slug(text string) string {
	text = normalizeDomainText(text)
	var out strings.Builder
	lastDash := false
	for _, r := range text {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			out.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			out.WriteByte('-')
			lastDash = true
		}
	}
	return strings.Trim(out.String(), "-")
}

func normalizeDomainText(text string) string {
	replacer := strings.NewReplacer(
		"á", "a", "à", "a", "ã", "a", "â", "a",
		"é", "e", "ê", "e",
		"í", "i",
		"ó", "o", "õ", "o", "ô", "o",
		"ú", "u",
		"ç", "c",
	)
	return replacer.Replace(strings.ToLower(text))
}

func (e *SymbolicEngine) ExecuteTask(t Task) (json.RawMessage, error) {
	if strings.TrimSpace(t.Description) == "" {
		return nil, fmt.Errorf("tarefa sem descricao")
	}
	e.mu.RLock()
	rules := e.matchingRulesLocked(t)
	e.mu.RUnlock()
	result := renderExecutionResult(t, rules)
	raw, err := json.Marshal(result)
	if err != nil {
		return nil, err
	}
	return raw, nil
}

func (e *SymbolicEngine) matchingRulesLocked(t Task) []Rule {
	haystack := strings.ToLower(t.Role + " " + t.Description + " " + string(t.Payload))
	return e.matchingRulesForTextLocked(haystack)
}

func (e *SymbolicEngine) matchingRulesForTextLocked(text string) []Rule {
	haystack := normalizeDomainText(text)
	var matched []Rule
	for _, rules := range e.rules {
		for _, rule := range rules {
			pattern := normalizeDomainText(strings.TrimSpace(rule.Pattern))
			if pattern == "" || strings.Contains(haystack, pattern) {
				matched = append(matched, rule)
			}
		}
	}
	sort.Slice(matched, func(i, j int) bool {
		if matched[i].Domain == matched[j].Domain {
			return matched[i].Pattern < matched[j].Pattern
		}
		return matched[i].Domain < matched[j].Domain
	})
	return matched
}

func renderReport(t Task, rules []Rule) string {
	result := renderExecutionResult(t, rules)
	return result.Report
}

func renderExecutionResult(t Task, rules []Rule) ExecutionResult {
	data := map[string]any{
		"ID":          t.ID,
		"Domain":      t.Domain,
		"Role":        t.Role,
		"Description": t.Description,
		"RuleCount":   len(rules),
		"Rules":       rules,
	}
	roleTemplate := nonEmptyLines(renderRoleTemplate(t, data))
	appliedRules := renderAppliedRules(rules, data)
	evidence := []string{
		"task_description:" + strings.TrimSpace(t.Description),
		"role:" + strings.TrimSpace(t.Role),
	}
	if strings.TrimSpace(t.Domain) != "" {
		evidence = append(evidence, "domain:"+strings.TrimSpace(t.Domain))
	}
	if len(t.Payload) > 0 {
		evidence = append(evidence, "input_payload:present")
	}
	nextActions := []string{
		"registrar resultado no SQLite",
		"preservar hash do contrato e handoffs",
		"submeter saida ao crivo tecnico",
	}
	var out bytes.Buffer
	out.WriteString("Relatorio de Estado\n")
	out.WriteString("Funcao: " + t.Role + "\n")
	out.WriteString("Instrucao: " + t.Description + "\n")
	if len(t.Payload) > 0 {
		out.WriteString("Estado recebido: ")
		out.Write(t.Payload)
		out.WriteString("\n")
	}
	out.WriteString("Template do Papel:\n")
	out.WriteString(renderRoleTemplate(t, data))
	out.WriteString("\n")
	out.WriteString(fmt.Sprintf("Regras aplicadas: %d\n", len(rules)))
	for _, rule := range rules {
		if strings.TrimSpace(rule.Template) == "" {
			out.WriteString("- " + rule.Domain + ": " + rule.Action + "\n")
			continue
		}
		tpl, err := template.New("rule").Parse(rule.Template)
		if err != nil {
			out.WriteString("- " + rule.Domain + ": " + rule.Action + "\n")
			continue
		}
		out.WriteString("- ")
		_ = tpl.Execute(&out, data)
		out.WriteString("\n")
	}
	out.WriteString("Saida: tarefa executada pelo Symbolic Engine deterministico do OBG v2.")
	return ExecutionResult{
		TaskID:        t.ID,
		Domain:        t.Domain,
		Role:          t.Role,
		Instruction:   t.Description,
		Status:        StatusDone,
		Summary:       "tarefa executada pelo Symbolic Engine deterministico do OBG v2",
		InputPayload:  cloneRawMessage(t.Payload),
		RoleTemplate:  roleTemplate,
		RulesApplied:  appliedRules,
		Evidence:      evidence,
		NextActions:   nextActions,
		Report:        out.String(),
		GeneratedBy:   "symbolic_engine",
		Deterministic: true,
	}
}

func renderAppliedRules(rules []Rule, data map[string]any) []AppliedRule {
	out := make([]AppliedRule, 0, len(rules))
	for _, rule := range rules {
		rendered := strings.TrimSpace(rule.Template)
		if rendered != "" {
			tpl, err := template.New("rule").Parse(rule.Template)
			if err == nil {
				var buf bytes.Buffer
				if tpl.Execute(&buf, data) == nil {
					rendered = strings.TrimSpace(buf.String())
				}
			}
		}
		if rendered == "" {
			rendered = strings.TrimSpace(rule.Action)
		}
		if rendered == "" {
			rendered = strings.TrimSpace(rule.Pattern)
		}
		out = append(out, AppliedRule{
			Domain:   rule.Domain,
			Pattern:  rule.Pattern,
			Action:   rule.Action,
			Rendered: rendered,
		})
	}
	return out
}

func nonEmptyLines(text string) []string {
	var out []string
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(strings.TrimPrefix(line, "-"))
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}

func cloneRawMessage(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return nil
	}
	clone := make([]byte, len(raw))
	copy(clone, raw)
	return clone
}

var roleTemplates = map[string]string{
	"gerente": `- Confirmar objetivo e limites do contrato: {{.Description}}.
- Registrar lacunas, premissas e criterio de aceite.
- Preservar hash, dependencias e trilha auditavel.`,
	"arquiteto": `- Decompor a entrega em modulos pequenos e ordenados.
- Explicitar dependencias, riscos tecnicos e validacoes.
- Produzir plano executavel para o dominio {{.Domain}}.`,
	"desenvolvedor": `- Implementar somente o escopo contratado.
- Gerar artefato verificavel e rastreavel.
- Registrar comandos, arquivos e evidencias tecnicas.`,
	"analista": `- Definir perguntas, fontes e criterios de avaliacao.
- Separar achados, evidencias, lacunas e riscos.
- Entregar conclusao rastreavel ao contrato.`,
	"redator": `- Estruturar mensagem, publico e objetivo.
- Produzir texto claro, verificavel e sem ruido narrativo.
- Revisar consistencia, completude e aderencia aos entregaveis.`,
	"auditor tecnico": `- Verificar contrato, hash, artefatos e handoffs.
- Reprovar saida sem evidencia, com loop ou fora do escopo.
- Selar somente resultado auditavel e read-only.`,
}

func renderRoleTemplate(task Task, data map[string]any) string {
	key := roleTemplateKey(task.Role)
	raw, ok := roleTemplates[key]
	if !ok {
		raw = `- Executar a tarefa com regras locais.
- Registrar resultado tecnico e evidencia.
- Manter aderencia ao contrato e aos handoffs recebidos.`
	}
	tpl, err := template.New("role").Parse(raw)
	if err != nil {
		return raw
	}
	var out bytes.Buffer
	if err := tpl.Execute(&out, data); err != nil {
		return raw
	}
	return out.String()
}

func roleTemplateKey(role string) string {
	normalized := normalizeDomainText(role)
	switch {
	case strings.Contains(normalized, "gerente"):
		return "gerente"
	case strings.Contains(normalized, "arquiteto"):
		return "arquiteto"
	case strings.Contains(normalized, "desenvolvedor"):
		return "desenvolvedor"
	case strings.Contains(normalized, "analista"):
		return "analista"
	case strings.Contains(normalized, "redator"):
		return "redator"
	case strings.Contains(normalized, "auditor"):
		return "auditor tecnico"
	default:
		return normalized
	}
}
