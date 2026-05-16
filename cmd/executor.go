package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"omni-bot-go/database"
)

type fileArtifact struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Bytes  int    `json:"bytes"`
}

type taskExecution struct {
	Report      string             `json:"report"`
	Artifacts   []fileArtifact     `json:"artifacts"`
	Validations []validationResult `json:"validations,omitempty"`
}

func (m *Manager) executeFileTask(task database.Task, validations []validationResult) (taskExecution, error) {
	contract, err := m.store.GetContract(task.ContractID)
	if err != nil {
		return taskExecution{}, err
	}
	baseDir := filepath.Join("projects", fmt.Sprintf("contract-%d", task.ContractID))
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return taskExecution{}, err
	}
	previous, err := m.previousReports(task)
	if err != nil {
		return taskExecution{}, err
	}

	var name string
	var data []byte
	switch {
	case strings.Contains(strings.ToLower(task.Role), "gerente"):
		name = "01-contract.md"
		data = []byte(renderContractMarkdown(contract))
	case strings.Contains(strings.ToLower(task.Role), "arquiteto"):
		name = "02-task-plan.json"
		data, err = json.MarshalIndent(map[string]any{
			"contract_id":    contract.ID,
			"north_star":     contract.NorthStar,
			"constraints":    contract.Constraints,
			"deliverables":   contract.Deliverables,
			"modules":        plannedModules(contract),
			"previous_state": previous,
			"generated_at":   time.Now().Format(time.RFC3339),
		}, "", "  ")
	case strings.Contains(strings.ToLower(task.Role), "operario"):
		name = "03-initial-delivery.md"
		data = []byte(renderInitialDelivery(contract, previous))
	default:
		name = "04-audit.json"
		if locked, err := m.store.ArtifactExists(filepath.Clean(filepath.Join(baseDir, name))); err != nil {
			return taskExecution{}, err
		} else if locked {
			name = fmt.Sprintf("04-audit-task-%d.json", task.ID)
		}
		data, err = json.MarshalIndent(map[string]any{
			"contract_id":        contract.ID,
			"audit":              "artefatos revisados e tarefa selada",
			"immutable_contract": contract.Hash,
			"validations":        validations,
			"previous_state":     previous,
			"validated_at":       time.Now().Format(time.RFC3339),
		}, "", "  ")
	}
	if err != nil {
		return taskExecution{}, err
	}
	artifact, err := m.writeSealedArtifact(task, baseDir, name, data)
	if err != nil {
		return taskExecution{}, err
	}
	return taskExecution{
		Report:      fmt.Sprintf("Arquivo real gerado e selado: %s", artifact.Path),
		Artifacts:   []fileArtifact{artifact},
		Validations: validations,
	}, nil
}

func (m *Manager) writeSealedArtifact(task database.Task, baseDir, name string, data []byte) (fileArtifact, error) {
	if len(strings.TrimSpace(string(data))) == 0 {
		return fileArtifact{}, errors.New("artefato vazio")
	}
	path := filepath.Clean(filepath.Join(baseDir, name))
	rel, err := filepath.Rel(baseDir, path)
	if err != nil || strings.HasPrefix(rel, "..") || filepath.IsAbs(rel) {
		return fileArtifact{}, errors.New("caminho de artefato fora do contrato")
	}
	locked, err := m.store.ArtifactExists(path)
	if err != nil {
		return fileArtifact{}, err
	}
	if locked {
		return fileArtifact{}, fmt.Errorf("artefato read-only ja existe: %s", path)
	}
	if err := os.WriteFile(path, data, 0444); err != nil {
		return fileArtifact{}, err
	}
	sum := sha256.Sum256(data)
	hash := hex.EncodeToString(sum[:])
	if err := m.store.AddArtifact(task.ContractID, task.ID, path, hash); err != nil {
		return fileArtifact{}, err
	}
	return fileArtifact{Path: path, SHA256: hash, Bytes: len(data)}, nil
}

func (m *Manager) previousReports(task database.Task) ([]map[string]any, error) {
	var out []map[string]any
	handoffs, err := m.store.HandoffsForTask(task)
	if err != nil {
		return nil, err
	}
	for _, handoff := range handoffs {
		out = append(out, map[string]any{
			"source":       "handoff_area",
			"handoff_id":   handoff.ID,
			"from_task_id": handoff.FromTaskID,
			"to_task_id":   handoff.ToTaskID,
			"kind":         handoff.Kind,
			"sha256":       handoff.SHA256,
			"payload":      handoff.Payload,
		})
	}
	for _, id := range task.Dependencies {
		dep, err := m.store.GetTask(id)
		if err != nil {
			return nil, err
		}
		out = append(out, map[string]any{
			"task_id": id,
			"status":  dep.Status,
			"payload": dep.Payload,
		})
	}
	return out, nil
}

func renderContractMarkdown(contract database.Contract) string {
	return fmt.Sprintf(`# Contrato de Balcao

Contrato: %d
Hash: %s

## North Star
%s

## Restricoes
%s

## Entregaveis
%s
`, contract.ID, contract.Hash, contract.NorthStar, contract.Constraints, contract.Deliverables)
}

func plannedModules(contract database.Contract) []map[string]string {
	return []map[string]string{
		{"name": "contrato", "owner": "Gerente", "goal": "preservar premissas e restricoes"},
		{"name": "planejamento", "owner": "Arquiteto", "goal": "dividir entrega em modulos auditaveis"},
		{"name": "execucao", "owner": "Operario", "goal": "produzir artefatos em arquivo"},
		{"name": "auditoria", "owner": "Auditor Tecnico", "goal": "validar e selar saidas"},
	}
}

func renderInitialDelivery(contract database.Contract, previous []map[string]any) string {
	raw, _ := json.MarshalIndent(previous, "", "  ")
	return fmt.Sprintf(`# Entrega Inicial

Contrato: %d

## Objetivo
%s

## Estado Recebido
%s

## Resultado
Entrega inicial materializada em arquivo pelo operario polimorfico.
`, contract.ID, contract.NorthStar, string(raw))
}
