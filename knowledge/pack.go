package knowledge

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type KnowledgePack struct {
	Domain    string    `json:"domain"`
	Version   string    `json:"version"`
	Rules     []Rule    `json:"rules"`
	LoadedAt  time.Time `json:"loaded_at"`
	ExpiresAt time.Time `json:"expires_at,omitempty"`
}

type Rule struct {
	Pattern  string `json:"pattern"`
	Template string `json:"template"`
	Action   string `json:"action"`
}

func CorePack() *KnowledgePack {
	now := time.Now()
	return &KnowledgePack{
		Domain:   "core",
		Version:  "2.0",
		LoadedAt: now,
		Rules: []Rule{
			{Pattern: "contrato", Action: "preserve_contract", Template: "Preservar contrato/hash antes de executar {{.Description}}."},
			{Pattern: "auditar", Action: "technical_gate", Template: "Aplicar crivo tecnico com evidencia e sem ruido narrativo."},
			{Pattern: "sqlite", Action: "solid_state", Template: "Persistir fatos no SQLite; se nao esta no DB, nao aconteceu."},
			{Pattern: "artefato", Action: "seal_artifact", Template: "Gerar artefato verificavel e selar com hash."},
		},
	}
}

func LoadFromFile(path string) (*KnowledgePack, error) {
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return nil, err
	}
	var pack KnowledgePack
	if err := json.Unmarshal(data, &pack); err != nil {
		return nil, err
	}
	if strings.TrimSpace(pack.Domain) == "" {
		return nil, fmt.Errorf("knowledge pack sem domain")
	}
	if pack.Version == "" {
		pack.Version = "1.0"
	}
	pack.LoadedAt = time.Now()
	return &pack, nil
}

func (p *KnowledgePack) Save(dir string) error {
	if p == nil {
		return fmt.Errorf("knowledge pack nil")
	}
	if strings.TrimSpace(p.Domain) == "" {
		return fmt.Errorf("knowledge pack sem domain")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(dir, fmt.Sprintf("%s.pack.json", p.Domain))
	return os.WriteFile(path, data, 0600)
}

func RuleMapToPack(ruleMap RuleMap) *KnowledgePack {
	pack := &KnowledgePack{
		Domain:    ruleMap.Topic,
		Version:   "jit",
		LoadedAt:  time.Now(),
		ExpiresAt: time.Now().Add(TTL),
	}
	if len(ruleMap.PackRules) > 0 {
		pack.Rules = append(pack.Rules, ruleMap.PackRules...)
		return pack
	}
	for _, action := range ruleMap.Rules {
		pack.Rules = append(pack.Rules, Rule{
			Pattern:  "",
			Action:   action,
			Template: action,
		})
	}
	return pack
}

func KnowledgePackToRuleMap(pack *KnowledgePack, source string) RuleMap {
	rules := map[string]string{}
	for i, rule := range pack.Rules {
		key := rule.Action
		if strings.TrimSpace(key) == "" {
			key = fmt.Sprintf("regra_%02d", i+1)
		}
		rules[key] = rule.Template
	}
	return RuleMap{
		Topic:     pack.Domain,
		Source:    source,
		Summary:   fmt.Sprintf("Knowledge Pack %s@%s com %d regra(s).", pack.Domain, pack.Version, len(pack.Rules)),
		Rules:     rules,
		PackRules: append([]Rule(nil), pack.Rules...),
		CreatedAt: time.Now(),
	}
}
