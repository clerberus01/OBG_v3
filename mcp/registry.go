package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"omni-bot-go/plugins/sandbox"
)

type Manifest struct {
	ID             string            `json:"id"`
	Name           string            `json:"name"`
	Description    string            `json:"description"`
	Transport      string            `json:"transport"`
	Command        string            `json:"command"`
	Args           []string          `json:"args"`
	Env            map[string]string `json:"env"`
	Endpoint       string            `json:"endpoint"`
	Headers        map[string]string `json:"headers"`
	Enabled        bool              `json:"enabled"`
	TimeoutSeconds int               `json:"timeout_seconds"`
	Sandbox        sandbox.Policy    `json:"sandbox"`
	Tools          []Tool            `json:"tools"`
}

type SandboxPolicy = sandbox.Policy

type Tool struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type Plugin struct {
	Manifest
	Path string `json:"path"`
}

type Registry struct {
	Dir     string
	Plugins []Plugin
}

type CommandRegistration struct {
	PluginID     string         `json:"plugin_id"`
	Tool         string         `json:"tool"`
	Kind         string         `json:"kind"`
	Transport    string         `json:"transport"`
	Target       string         `json:"target"`
	Enabled      bool           `json:"enabled"`
	ManifestPath string         `json:"manifest_path"`
	Sandbox      sandbox.Policy `json:"sandbox"`
}

type CallRequest struct {
	PluginID    string         `json:"plugin_id"`
	Tool        string         `json:"tool"`
	Input       map[string]any `json:"input"`
	ContractID  int64          `json:"contract_id,omitempty"`
	TaskID      int64          `json:"task_id,omitempty"`
	Permissions sandbox.Policy `json:"permissions,omitempty"`
}

type CallResult struct {
	PluginID   string `json:"plugin_id"`
	Tool       string `json:"tool"`
	Output     string `json:"output"`
	Duration   string `json:"duration"`
	Sandboxed  bool   `json:"sandboxed"`
	WorkDir    string `json:"work_dir,omitempty"`
	Transport  string `json:"transport,omitempty"`
	ContractID int64  `json:"contract_id,omitempty"`
	TaskID     int64  `json:"task_id,omitempty"`
}

func LoadRegistry(dir string) (Registry, error) {
	if strings.TrimSpace(dir) == "" {
		dir = "plugins"
	}
	registry := Registry{Dir: dir}
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return registry, nil
	}
	if err != nil {
		return registry, err
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".json") {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		plugin, err := LoadManifest(path)
		if err != nil {
			return registry, err
		}
		registry.Plugins = append(registry.Plugins, plugin)
	}
	return registry, nil
}

func LoadManifest(path string) (Plugin, error) {
	raw, err := os.ReadFile(path) // #nosec G304 -- path is built from registry directory entries or explicit local manifest path.
	if err != nil {
		return Plugin{}, err
	}
	var manifest Manifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return Plugin{}, fmt.Errorf("%s: %w", path, err)
	}
	plugin := Plugin{Manifest: manifest, Path: path}
	if err := plugin.Validate(); err != nil {
		return Plugin{}, fmt.Errorf("%s: %w", path, err)
	}
	return plugin, nil
}

func (p Plugin) Validate() error {
	if strings.TrimSpace(p.ID) == "" {
		return errors.New("plugin sem id")
	}
	if strings.TrimSpace(p.Name) == "" {
		return errors.New("plugin sem name")
	}
	if p.Transport == "" {
		p.Transport = "stdio-json"
	}
	if !sandbox.SupportedTransport(p.Transport) {
		return fmt.Errorf("transport nao suportado: %s", p.Transport)
	}
	if p.Transport == "stdio-json" {
		if strings.TrimSpace(p.Command) == "" {
			return errors.New("plugin sem command")
		}
		if strings.ContainsAny(p.Command, "&|;<>\n\r") {
			return errors.New("command deve ser executavel direto, sem shell")
		}
		for _, arg := range p.Args {
			if err := sandbox.ValidateProcessArg(arg); err != nil {
				return err
			}
		}
	} else if err := sandbox.ValidateServiceEndpoint(p.Transport, p.Endpoint); err != nil {
		return err
	}
	if err := sandbox.ValidateHeaders(p.Headers); err != nil {
		return err
	}
	if p.TimeoutSeconds < 0 || p.TimeoutSeconds > 300 {
		return errors.New("timeout_seconds deve ficar entre 0 e 300")
	}
	if err := p.validateSandbox(); err != nil {
		return err
	}
	seen := map[string]struct{}{}
	for _, tool := range p.Tools {
		if strings.TrimSpace(tool.Name) == "" {
			return errors.New("tool sem name")
		}
		if _, ok := seen[tool.Name]; ok {
			return fmt.Errorf("tool duplicada: %s", tool.Name)
		}
		seen[tool.Name] = struct{}{}
	}
	return nil
}

func (r Registry) Find(id string) (Plugin, bool) {
	for _, plugin := range r.Plugins {
		if plugin.ID == id {
			return plugin, true
		}
	}
	return Plugin{}, false
}

func (r Registry) CommandRegistrations() []CommandRegistration {
	var out []CommandRegistration
	for _, plugin := range r.Plugins {
		out = append(out, plugin.CommandRegistrations()...)
	}
	return out
}

func (p Plugin) CommandRegistrations() []CommandRegistration {
	tools := p.Tools
	if len(tools) == 0 {
		tools = []Tool{{Name: "*", Description: "qualquer ferramenta declarada pelo servico"}}
	}
	out := make([]CommandRegistration, 0, len(tools))
	for _, tool := range tools {
		out = append(out, CommandRegistration{
			PluginID:     p.ID,
			Tool:         tool.Name,
			Kind:         commandKind(p.Transport),
			Transport:    p.Transport,
			Target:       commandTarget(p),
			Enabled:      p.Enabled,
			ManifestPath: p.Path,
			Sandbox:      p.Sandbox,
		})
	}
	return out
}

func (r Registry) Call(ctx context.Context, req CallRequest) (CallResult, error) {
	plugin, ok := r.Find(req.PluginID)
	if !ok {
		return CallResult{}, fmt.Errorf("plugin nao encontrado: %s", req.PluginID)
	}
	return plugin.Call(ctx, req)
}

func commandKind(transport string) string {
	switch transport {
	case "local-service":
		return "local-service"
	case "web-service":
		return "web-service"
	default:
		return "local-command"
	}
}

func commandTarget(p Plugin) string {
	if p.Transport == "local-service" || p.Transport == "web-service" {
		return p.Endpoint
	}
	return strings.TrimSpace(strings.Join(append([]string{p.Command}, p.Args...), " "))
}

func (p Plugin) Call(ctx context.Context, req CallRequest) (CallResult, error) {
	if !p.Enabled {
		return CallResult{}, fmt.Errorf("plugin desabilitado: %s", p.ID)
	}
	tool := req.Tool
	input := req.Input
	if strings.TrimSpace(tool) == "" {
		return CallResult{}, errors.New("tool ausente")
	}
	if len(p.Tools) > 0 && !p.hasTool(tool) {
		return CallResult{}, fmt.Errorf("tool %q nao declarada pelo plugin %s", tool, p.ID)
	}
	timeout := time.Duration(p.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	callCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	payload, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      time.Now().UnixNano(),
		"method":  "tools/call",
		"params": map[string]any{
			"name":      tool,
			"arguments": input,
		},
	})
	if err != nil {
		return CallResult{}, err
	}
	started := time.Now()
	if p.Transport != "stdio-json" {
		return p.callService(callCtx, req, payload, started, timeout)
	}
	runResult, err := sandbox.Run(callCtx, sandbox.Request{
		PluginID:     p.ID,
		ManifestPath: p.Path,
		Command:      p.Command,
		Args:         p.Args,
		Env:          p.Env,
		Policy:       p.Sandbox,
		Permissions:  req.Permissions,
		ContractID:   req.ContractID,
		TaskID:       req.TaskID,
	}, string(payload)+"\n")
	result := CallResult{
		PluginID:   p.ID,
		Tool:       tool,
		Output:     runResult.Output,
		Duration:   nonEmpty(runResult.Duration, time.Since(started).String()),
		Sandboxed:  true,
		WorkDir:    runResult.WorkDir,
		Transport:  p.Transport,
		ContractID: req.ContractID,
		TaskID:     req.TaskID,
	}
	if callCtx.Err() == context.DeadlineExceeded {
		return result, fmt.Errorf("plugin timeout apos %s", timeout)
	}
	if err != nil {
		return result, err
	}
	return result, nil
}

func (p Plugin) callService(ctx context.Context, req CallRequest, payload []byte, started time.Time, timeout time.Duration) (CallResult, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.Endpoint, bytes.NewReader(payload))
	if err != nil {
		return CallResult{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("X-OBG-Plugin-ID", p.ID)
	httpReq.Header.Set("X-OBG-Tool", req.Tool)
	if req.ContractID > 0 {
		httpReq.Header.Set("X-OBG-Contract-ID", fmt.Sprint(req.ContractID))
	}
	if req.TaskID > 0 {
		httpReq.Header.Set("X-OBG-Task-ID", fmt.Sprint(req.TaskID))
	}
	for key, value := range p.Headers {
		httpReq.Header.Set(key, value)
	}
	client := &http.Client{Timeout: timeout}
	resp, err := client.Do(httpReq)
	result := CallResult{
		PluginID:   p.ID,
		Tool:       req.Tool,
		Duration:   time.Since(started).String(),
		Sandboxed:  true,
		Transport:  p.Transport,
		ContractID: req.ContractID,
		TaskID:     req.TaskID,
	}
	if ctx.Err() == context.DeadlineExceeded {
		return result, fmt.Errorf("plugin timeout apos %s", timeout)
	}
	if err != nil {
		return result, err
	}
	defer resp.Body.Close()
	body, readErr := io.ReadAll(io.LimitReader(resp.Body, int64(nonZero(p.Sandbox.MaxOutputBytes, 16000))+1))
	result.Output = sandbox.TrimOutput(string(body), nonZero(p.Sandbox.MaxOutputBytes, 16000))
	if readErr != nil {
		return result, readErr
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return result, fmt.Errorf("servico plugin respondeu HTTP %d", resp.StatusCode)
	}
	return result, nil
}

func (p Plugin) hasTool(name string) bool {
	for _, tool := range p.Tools {
		if tool.Name == name {
			return true
		}
	}
	return false
}

func (p Plugin) validateSandbox() error {
	return sandbox.ValidatePolicy(p.Sandbox, p.Env)
}

func nonEmpty(value, fallback string) string {
	if strings.TrimSpace(value) != "" {
		return value
	}
	return fallback
}

func nonZero(value, fallback int) int {
	if value > 0 {
		return value
	}
	return fallback
}
