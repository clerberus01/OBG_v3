package engine

import (
	"encoding/json"
	"errors"

	"omni-bot-go/engine/symbolic"
	"omni-bot-go/knowledge"
)

type Engine struct {
	symbolic *symbolic.SymbolicEngine
}

type Result struct {
	Text          string          `json:"text"`
	Structured    json.RawMessage `json:"structured,omitempty"`
	EngineHash    string          `json:"engine_hash"`
	Deterministic bool            `json:"deterministic"`
}

type Info struct {
	Name        string              `json:"name"`
	Mode        string              `json:"mode"`
	Version     string              `json:"version"`
	RuleCount   int                 `json:"rule_count"`
	PackCount   int                 `json:"pack_count"`
	Packs       []symbolic.PackInfo `json:"packs"`
	Description string              `json:"description"`
}

func New() (*Engine, error) {
	core := symbolic.NewSymbolicEngine()
	core.LoadKnowledgePack(knowledge.CorePack())
	return &Engine{symbolic: core}, nil
}

func (e *Engine) Close() error {
	return nil
}

func (e *Engine) Info() Info {
	info := e.symbolic.Info()
	return Info{
		Name:        "Omni-Bot Go Symbolic Engine",
		Mode:        "symbolic",
		Version:     "2.0",
		RuleCount:   info.RuleCount,
		PackCount:   len(info.Packs),
		Packs:       info.Packs,
		Description: "Motor deterministico baseado em regras, templates e contratos selados.",
	}
}

func (e *Engine) LoadKnowledgePack(pack *knowledge.KnowledgePack) {
	e.symbolic.LoadKnowledgePack(pack)
}

func (e *Engine) Plan(contract any) ([]symbolic.Task, error) {
	raw, err := json.Marshal(contract)
	if err != nil {
		return nil, err
	}
	return e.symbolic.PlanHierarchical(raw)
}

func (e *Engine) Infer(role, instruction, stateJSON string) (Result, error) {
	if instruction == "" {
		return Result{}, errors.New("instrucao vazia")
	}
	output, err := e.symbolic.ExecuteTask(symbolic.Task{
		ID:          "runtime-task",
		Description: instruction,
		Role:        role,
		Payload:     json.RawMessage(stateJSON),
		Status:      symbolic.StatusPending,
	})
	if err != nil {
		return Result{}, err
	}
	text := string(output)
	var legacyText string
	if err := json.Unmarshal(output, &legacyText); err == nil {
		text = legacyText
	} else {
		var structured struct {
			Report  string `json:"report"`
			Summary string `json:"summary"`
		}
		if err := json.Unmarshal(output, &structured); err == nil {
			switch {
			case structured.Report != "":
				text = structured.Report
			case structured.Summary != "":
				text = structured.Summary
			}
		}
	}
	return Result{
		Text:          text,
		Structured:    output,
		EngineHash:    e.symbolic.Hash(),
		Deterministic: true,
	}, nil
}
