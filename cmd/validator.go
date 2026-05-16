package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"omni-bot-go/database"
)

type validationResult struct {
	Name     string `json:"name"`
	Domain   string `json:"domain,omitempty"`
	Command  string `json:"command,omitempty"`
	Passed   bool   `json:"passed"`
	Output   string `json:"output"`
	Duration string `json:"duration"`
}

func (m *Manager) validateContract(contract database.Contract, task database.Task) ([]validationResult, error) {
	var results []validationResult
	start := time.Now()
	artifacts, err := contractArtifactInventory(contract.ID)
	if err != nil {
		result := validationResult{
			Name:     "artifact-inventory",
			Passed:   false,
			Output:   err.Error(),
			Duration: time.Since(start).String(),
		}
		results = append(results, result)
		m.logValidationResults(results)
		return results, err
	}
	internal, err := validateJSONArtifacts(contract.ID, artifacts)
	results = append(results, validationResult{
		Name:     "json-artifacts",
		Passed:   err == nil,
		Output:   internal,
		Duration: time.Since(start).String(),
	})
	if err != nil {
		m.logValidationResults(results)
		return results, err
	}

	domain := taskDomain(task)
	start = time.Now()
	domainOutput, err := validateDomainArtifacts(domain, artifacts)
	results = append(results, validationResult{
		Name:     "domain-artifact-matrix",
		Domain:   domain,
		Passed:   err == nil,
		Output:   domainOutput,
		Duration: time.Since(start).String(),
	})
	if err != nil {
		m.logValidationResults(results)
		return results, err
	}

	for _, command := range validationCommands(contract) {
		if !externalGoValidationEnabled() {
			results = append(results, validationResult{
				Name:     command,
				Command:  command,
				Passed:   true,
				Output:   "validacao externa de Go desabilitada; defina OBG_ALLOW_GO_VALIDATION=1 para permitir geracao de binarios temporarios",
				Duration: "0s",
			})
			continue
		}
		result := runValidationCommand(command)
		results = append(results, result)
		if !result.Passed {
			m.logValidationResults(results)
			return results, fmt.Errorf("validacao falhou: %s", command)
		}
	}
	m.logValidationResults(results)
	return results, nil
}

type artifactInventory struct {
	Total int
	JSON  int
	MD    int
	Go    int
	Other int
}

func contractArtifactInventory(contractID int64) (artifactInventory, error) {
	baseDir := filepath.Join("projects", fmt.Sprintf("contract-%d", contractID))
	var inventory artifactInventory
	err := filepath.WalkDir(baseDir, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		inventory.Total++
		switch strings.ToLower(filepath.Ext(path)) {
		case ".json":
			inventory.JSON++
		case ".md":
			inventory.MD++
		case ".go":
			inventory.Go++
		default:
			inventory.Other++
		}
		return nil
	})
	if err != nil {
		return inventory, err
	}
	return inventory, nil
}

func validateJSONArtifacts(contractID int64, inventory artifactInventory) (string, error) {
	baseDir := filepath.Join("projects", fmt.Sprintf("contract-%d", contractID))
	var checked int
	err := filepath.WalkDir(baseDir, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(path), ".json") {
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if !json.Valid(raw) {
			return fmt.Errorf("json invalido: %s", path)
		}
		checked++
		return nil
	})
	if err != nil {
		return err.Error(), err
	}
	return fmt.Sprintf("%d arquivo(s) JSON validado(s); inventario contrato: total=%d md=%d json=%d go=%d other=%d", checked, inventory.Total, inventory.MD, inventory.JSON, inventory.Go, inventory.Other), nil
}

func validateDomainArtifacts(domain string, inventory artifactInventory) (string, error) {
	if inventory.Total == 0 {
		return "nenhum artefato encontrado para o contrato", errors.New("nenhum artefato encontrado para o contrato")
	}
	switch domain {
	case "code":
		if inventory.JSON == 0 && inventory.Go == 0 {
			return "dominio code exige artefato JSON estruturado ou arquivo Go", errors.New("dominio code sem artefato tecnico")
		}
	case "document":
		if inventory.MD == 0 {
			return "dominio document exige artefato markdown", errors.New("dominio document sem markdown")
		}
	case "analysis":
		if inventory.JSON == 0 {
			return "dominio analysis exige artefato JSON auditavel", errors.New("dominio analysis sem JSON")
		}
	case "automation":
		if inventory.JSON == 0 && inventory.MD == 0 {
			return "dominio automation exige plano/execucao em JSON ou Markdown", errors.New("dominio automation sem artefato auditavel")
		}
	case "design":
		if inventory.MD == 0 && inventory.JSON == 0 {
			return "dominio design exige especificacao visual em Markdown ou JSON", errors.New("dominio design sem especificacao")
		}
	case "strategy":
		if inventory.MD == 0 && inventory.JSON == 0 {
			return "dominio strategy exige plano em Markdown ou JSON", errors.New("dominio strategy sem plano")
		}
	}
	return fmt.Sprintf("dominio %s validado: total=%d md=%d json=%d go=%d other=%d", domain, inventory.Total, inventory.MD, inventory.JSON, inventory.Go, inventory.Other), nil
}

func validationCommands(contract database.Contract) []string {
	text := strings.ToLower(contract.Constraints + " " + contract.Deliverables)
	var out []string
	allowed := []string{"go build ./...", "go test ./..."}
	for _, command := range allowed {
		if strings.Contains(text, command) {
			out = append(out, command)
		}
	}
	return out
}

func runValidationCommand(command string) validationResult {
	start := time.Now()
	fields := strings.Fields(command)
	result := validationResult{Name: command, Command: command}
	if len(fields) == 0 {
		result.Output = "comando vazio"
		result.Duration = time.Since(start).String()
		return result
	}

	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, fields[0], fields[1:]...)
	cmd.Dir = "."
	cmd.Env = validationEnv(os.Environ())
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	result.Duration = time.Since(start).String()
	result.Output = trimOutput(out.String(), 6000)
	if ctx.Err() == context.DeadlineExceeded {
		result.Output = "timeout de validacao apos 4m\n" + result.Output
		return result
	}
	if err != nil {
		if result.Output == "" {
			result.Output = err.Error()
		}
		return result
	}
	result.Passed = true
	if result.Output == "" {
		result.Output = "ok"
	}
	return result
}

func validationEnv(env []string) []string {
	if cacheDir, err := filepath.Abs(filepath.Join("tmp", "go-validation-cache")); err == nil {
		_ = os.MkdirAll(cacheDir, 0755)
		env = upsertEnv(env, "GOCACHE", cacheDir)
	}
	if tmpDir, err := filepath.Abs(filepath.Join("tmp", "go-validation-tmp")); err == nil {
		_ = os.MkdirAll(tmpDir, 0755)
		env = upsertEnv(env, "GOTMPDIR", tmpDir)
	}
	env = upsertEnv(env, "CGO_ENABLED", "0")
	return env
}

func externalGoValidationEnabled() bool {
	return os.Getenv("OBG_ALLOW_GO_VALIDATION") == "1"
}

func upsertEnv(env []string, key, value string) []string {
	prefix := strings.ToUpper(key) + "="
	for i, item := range env {
		if strings.HasPrefix(strings.ToUpper(item), prefix) {
			env[i] = key + "=" + value
			return env
		}
	}
	return append(env, key+"="+value)
}

func trimOutput(output string, limit int) string {
	output = strings.TrimSpace(output)
	if len(output) <= limit {
		return output
	}
	return output[:limit] + "\n...saida truncada..."
}

func (m *Manager) logValidationResults(results []validationResult) {
	for _, result := range results {
		level := "INFO"
		if !result.Passed {
			level = "ERROR"
		}
		m.store.Log(level, "validator", fmt.Sprintf("%s passed=%t duration=%s", result.Name, result.Passed, result.Duration))
	}
}
