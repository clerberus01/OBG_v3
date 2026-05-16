package main

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"omni-bot-go/database"
)

type CommandBlueprint struct {
	NorthStar    string
	Constraints  string
	Deliverables string
	Tasks        []CommandTask
}

type CommandTask struct {
	Key        string
	Title      string
	Role       string
	DependsOn  []string
	Payload    map[string]any
	payloadKey string
}

func (m *Manager) LoadBlueprintFile(path string) (database.Contract, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return database.Contract{}, err
	}
	contract, err := m.CreateProjectFromYAML(string(raw))
	if err != nil {
		return database.Contract{}, err
	}
	m.store.Log("INFO", "yaml", fmt.Sprintf("blueprint carregado de %s para contrato %d", path, contract.ID))
	return contract, nil
}

func (m *Manager) CreateProjectFromYAML(raw string) (database.Contract, error) {
	blueprint, err := ParseCommandBlueprintYAML(raw)
	if err != nil {
		return database.Contract{}, err
	}
	return m.createContractFromBlueprint(blueprint)
}

func (m *Manager) createContractFromBlueprint(blueprint CommandBlueprint) (database.Contract, error) {
	contract, err := m.store.CreateContract(blueprint.NorthStar, blueprint.Constraints, blueprint.Deliverables)
	if err != nil {
		return database.Contract{}, err
	}
	taskIDs := map[string]int64{}
	for index, task := range blueprint.Tasks {
		var deps []int64
		for _, key := range task.DependsOn {
			id, ok := taskIDs[key]
			if !ok {
				return database.Contract{}, fmt.Errorf("tarefa %q depende de %q, que ainda nao foi declarada", task.Key, key)
			}
			deps = append(deps, id)
		}
		payload := map[string]any{
			"source": "yaml",
			"key":    task.Key,
		}
		for key, value := range task.Payload {
			payload[key] = value
		}
		created, err := m.store.AddTaskWithPayload(contract.ID, task.Title, task.Role, deps, payload)
		if err != nil {
			return database.Contract{}, err
		}
		key := task.Key
		if key == "" {
			key = fmt.Sprintf("task_%d", index+1)
		}
		taskIDs[key] = created.ID
	}
	m.store.Log("INFO", "yaml", fmt.Sprintf("contrato %d criado por YAML com %d tarefas", contract.ID, len(blueprint.Tasks)))
	m.bus.Publish("snapshot", m.Snapshot())
	return contract, nil
}

func ParseCommandBlueprintYAML(raw string) (CommandBlueprint, error) {
	var blueprint CommandBlueprint
	var listTarget string
	var current *CommandTask
	var payloadIndent = -1
	lines := strings.Split(raw, "\n")
	for lineNo, rawLine := range lines {
		line := stripYAMLComment(strings.TrimRight(rawLine, "\r\t "))
		if strings.TrimSpace(line) == "" {
			continue
		}
		indent := countIndent(line)
		text := strings.TrimSpace(line)
		if indent == 0 {
			payloadIndent = -1
		}
		if payloadIndent >= 0 && current != nil && indent > payloadIndent {
			key, value, ok := splitYAMLKeyValue(text)
			if !ok {
				return CommandBlueprint{}, fmt.Errorf("linha %d: payload invalido", lineNo+1)
			}
			current.Payload[key] = parseYAMLValue(value)
			continue
		}
		if strings.HasPrefix(text, "- ") {
			item := strings.TrimSpace(strings.TrimPrefix(text, "- "))
			switch listTarget {
			case "constraints":
				blueprint.Constraints = appendSentence(blueprint.Constraints, parseYAMLScalar(item))
			case "deliverables":
				blueprint.Deliverables = appendSentence(blueprint.Deliverables, parseYAMLScalar(item))
			case "tasks":
				blueprint.Tasks = append(blueprint.Tasks, CommandTask{Payload: map[string]any{}})
				current = &blueprint.Tasks[len(blueprint.Tasks)-1]
				if item != "" {
					key, value, ok := splitYAMLKeyValue(item)
					if !ok {
						return CommandBlueprint{}, fmt.Errorf("linha %d: item de tarefa invalido", lineNo+1)
					}
					applyTaskField(current, key, value)
				}
			default:
				return CommandBlueprint{}, fmt.Errorf("linha %d: lista sem campo pai", lineNo+1)
			}
			continue
		}
		key, value, ok := splitYAMLKeyValue(text)
		if !ok {
			return CommandBlueprint{}, fmt.Errorf("linha %d: campo YAML invalido", lineNo+1)
		}
		if indent == 0 {
			listTarget = ""
			switch key {
			case "north_star", "objetivo":
				blueprint.NorthStar = parseYAMLScalar(value)
			case "constraints", "restrictions", "restricoes":
				if value == "" {
					listTarget = "constraints"
				} else {
					blueprint.Constraints = parseYAMLScalar(value)
				}
			case "deliverables", "entregaveis":
				if value == "" {
					listTarget = "deliverables"
				} else {
					blueprint.Deliverables = parseYAMLScalar(value)
				}
			case "tasks", "tarefas":
				listTarget = "tasks"
			default:
				return CommandBlueprint{}, fmt.Errorf("linha %d: campo raiz desconhecido %q", lineNo+1, key)
			}
			continue
		}
		if listTarget != "tasks" || current == nil {
			return CommandBlueprint{}, fmt.Errorf("linha %d: campo fora de tarefa", lineNo+1)
		}
		if key == "payload" || key == "context" || key == "manual" {
			payloadIndent = indent
			if current.Payload == nil {
				current.Payload = map[string]any{}
			}
			if value != "" {
				current.Payload[key] = parseYAMLValue(value)
			}
			continue
		}
		applyTaskField(current, key, value)
	}
	if err := blueprint.validate(); err != nil {
		return CommandBlueprint{}, err
	}
	return blueprint, nil
}

func (b CommandBlueprint) validate() error {
	if strings.TrimSpace(b.NorthStar) == "" {
		return errors.New("YAML precisa de north_star")
	}
	if strings.TrimSpace(b.Constraints) == "" {
		return errors.New("YAML precisa de constraints")
	}
	if strings.TrimSpace(b.Deliverables) == "" {
		return errors.New("YAML precisa de deliverables")
	}
	if len(b.Tasks) == 0 {
		return errors.New("YAML precisa de pelo menos uma tarefa")
	}
	seen := map[string]struct{}{}
	for i, task := range b.Tasks {
		if strings.TrimSpace(task.Title) == "" {
			return fmt.Errorf("tarefa %d sem title", i+1)
		}
		if strings.TrimSpace(task.Role) == "" {
			return fmt.Errorf("tarefa %d sem role", i+1)
		}
		if task.Key != "" {
			if _, ok := seen[task.Key]; ok {
				return fmt.Errorf("id de tarefa duplicado: %s", task.Key)
			}
			seen[task.Key] = struct{}{}
		}
	}
	return nil
}

func applyTaskField(task *CommandTask, key string, value string) {
	switch key {
	case "id", "key":
		task.Key = parseYAMLScalar(value)
	case "title", "titulo":
		task.Title = parseYAMLScalar(value)
	case "role", "papel":
		task.Role = parseYAMLScalar(value)
	case "depends_on", "depends", "dependencias":
		task.DependsOn = parseYAMLStringList(value)
	default:
		if task.Payload == nil {
			task.Payload = map[string]any{}
		}
		task.Payload[key] = parseYAMLValue(value)
	}
}

func splitYAMLKeyValue(text string) (string, string, bool) {
	idx := strings.Index(text, ":")
	if idx <= 0 {
		return "", "", false
	}
	return strings.TrimSpace(text[:idx]), strings.TrimSpace(text[idx+1:]), true
}

func stripYAMLComment(line string) string {
	inQuote := rune(0)
	for i, r := range line {
		if r == '"' || r == '\'' {
			if inQuote == 0 {
				inQuote = r
			} else if inQuote == r {
				inQuote = 0
			}
			continue
		}
		if r == '#' && inQuote == 0 {
			return strings.TrimRight(line[:i], " \t")
		}
	}
	return line
}

func countIndent(line string) int {
	n := 0
	for _, r := range line {
		if r != ' ' {
			return n
		}
		n++
	}
	return n
}

func parseYAMLValue(value string) any {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, "[") && strings.HasSuffix(value, "]") {
		items := parseYAMLStringList(value)
		out := make([]any, len(items))
		for i, item := range items {
			out[i] = item
		}
		return out
	}
	switch strings.ToLower(value) {
	case "true":
		return true
	case "false":
		return false
	}
	return parseYAMLScalar(value)
}

func parseYAMLStringList(value string) []string {
	value = strings.TrimSpace(value)
	if value == "" || value == "[]" {
		return nil
	}
	if strings.HasPrefix(value, "[") && strings.HasSuffix(value, "]") {
		value = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(value, "["), "]"))
	}
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		item := parseYAMLScalar(part)
		if item != "" {
			out = append(out, item)
		}
	}
	return out
}

func parseYAMLScalar(value string) string {
	value = strings.TrimSpace(value)
	if len(value) >= 2 {
		if (value[0] == '"' && value[len(value)-1] == '"') || (value[0] == '\'' && value[len(value)-1] == '\'') {
			return value[1 : len(value)-1]
		}
	}
	return value
}

func appendSentence(base, item string) string {
	item = strings.TrimSpace(item)
	if item == "" {
		return base
	}
	if strings.TrimSpace(base) == "" {
		return item
	}
	return strings.TrimSpace(base) + " " + item
}
