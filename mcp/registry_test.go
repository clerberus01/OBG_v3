package mcp

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadRegistry(t *testing.T) {
	dir := t.TempDir()
	writeManifest(t, dir, Manifest{
		ID:        "fixture",
		Name:      "Fixture",
		Transport: "stdio-json",
		Command:   commandForTest(),
		Args:      argsForTest(),
		Env:       envForTest(),
		Enabled:   true,
		Sandbox:   sandboxForTest(),
		Tools:     []Tool{{Name: "echo", Description: "echo test"}},
	})
	registry, err := LoadRegistry(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(registry.Plugins) != 1 || registry.Plugins[0].ID != "fixture" {
		t.Fatalf("registry = %#v", registry)
	}
}

func TestCallPluginStdioJSON(t *testing.T) {
	dir := t.TempDir()
	manifest := Manifest{
		ID:        "fixture",
		Name:      "Fixture",
		Transport: "stdio-json",
		Command:   commandForTest(),
		Args:      argsForTest(),
		Env:       envForTest(),
		Enabled:   true,
		Sandbox:   sandboxForTest(),
		Tools:     []Tool{{Name: "echo", Description: "echo test"}},
	}
	writeManifest(t, dir, manifest)
	registry, err := LoadRegistry(dir)
	if err != nil {
		t.Fatal(err)
	}
	result, err := registry.Call(context.Background(), CallRequest{
		PluginID: "fixture",
		Tool:     "echo",
		Input:    map[string]any{"value": "ok"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.Output, "tools/call") || !strings.Contains(result.Output, "ok") {
		t.Fatalf("output = %q", result.Output)
	}
	if !result.Sandboxed || result.WorkDir == "" {
		t.Fatalf("sandbox metadata missing: %#v", result)
	}
}

func TestDisabledPluginCannotRun(t *testing.T) {
	plugin := Plugin{Manifest: Manifest{ID: "p", Name: "P", Transport: "stdio-json", Command: commandForTest(), Enabled: false}}
	_, err := plugin.Call(context.Background(), CallRequest{Tool: "x"})
	if err == nil || !strings.Contains(err.Error(), "desabilitado") {
		t.Fatalf("expected disabled error, got %v", err)
	}
}

func TestSandboxBlocksCommandOutsideAllowlist(t *testing.T) {
	dir := t.TempDir()
	plugin := Plugin{
		Manifest: Manifest{
			ID:        "p",
			Name:      "P",
			Transport: "stdio-json",
			Command:   commandForTest(),
			Args:      argsForTest(),
			Env:       envForTest(),
			Enabled:   true,
			Sandbox:   SandboxPolicy{AllowCommands: []string{"definitely-not-this-command"}, AllowEnv: []string{"OBG_PLUGIN_HELPER"}},
		},
		Path: filepath.Join(dir, "p.json"),
	}
	_, err := plugin.Call(context.Background(), CallRequest{Tool: "x"})
	if err == nil || !strings.Contains(err.Error(), "sandbox bloqueou command") {
		t.Fatalf("expected sandbox command error, got %v", err)
	}
}

func TestSandboxBlocksUndeclaredEnv(t *testing.T) {
	dir := t.TempDir()
	plugin := Plugin{
		Manifest: Manifest{
			ID:        "p",
			Name:      "P",
			Transport: "stdio-json",
			Command:   commandForTest(),
			Args:      argsForTest(),
			Env: map[string]string{
				"OBG_PLUGIN_HELPER": "1",
				"SECRET_TOKEN":      "nope",
			},
			Enabled: true,
			Sandbox: SandboxPolicy{AllowCommands: []string{commandForTest()}, AllowEnv: []string{"OBG_PLUGIN_HELPER"}},
		},
		Path: filepath.Join(dir, "p.json"),
	}
	_, err := plugin.Call(context.Background(), CallRequest{Tool: "x"})
	if err == nil || !strings.Contains(err.Error(), "sandbox bloqueou env SECRET_TOKEN") {
		t.Fatalf("expected sandbox env error, got %v", err)
	}
}

func TestCallPluginAppliesContractTaskPermissions(t *testing.T) {
	dir := t.TempDir()
	writeManifest(t, dir, Manifest{
		ID:        "fixture",
		Name:      "Fixture",
		Transport: "stdio-json",
		Command:   commandForTest(),
		Args:      argsForTest(),
		Env:       envForTest(),
		Enabled:   true,
		Sandbox:   sandboxForTest(),
		Tools:     []Tool{{Name: "echo", Description: "echo test"}},
	})
	registry, err := LoadRegistry(dir)
	if err != nil {
		t.Fatal(err)
	}
	result, err := registry.Call(context.Background(), CallRequest{
		PluginID:   "fixture",
		Tool:       "echo",
		ContractID: 7,
		TaskID:     9,
		Input:      map[string]any{"value": "ok"},
		Permissions: SandboxPolicy{
			AllowCommands:  []string{commandForTest()},
			AllowEnv:       []string{"OBG_PLUGIN_HELPER"},
			MaxOutputBytes: 64,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.ContractID != 7 || result.TaskID != 9 {
		t.Fatalf("scope missing: %#v", result)
	}
	if !strings.Contains(result.WorkDir, filepath.Join("contract-7", "task-9")) {
		t.Fatalf("workdir missing contract/task scope: %q", result.WorkDir)
	}
}

func TestCallPluginPermissionNarrowsCommand(t *testing.T) {
	dir := t.TempDir()
	plugin := Plugin{
		Manifest: Manifest{
			ID:        "p",
			Name:      "P",
			Transport: "stdio-json",
			Command:   commandForTest(),
			Args:      argsForTest(),
			Env:       envForTest(),
			Enabled:   true,
			Sandbox:   sandboxForTest(),
		},
		Path: filepath.Join(dir, "p.json"),
	}
	_, err := plugin.Call(context.Background(), CallRequest{
		Tool: "x",
		Permissions: SandboxPolicy{
			AllowCommands: []string{"gofmt"},
			AllowEnv:      []string{"OBG_PLUGIN_HELPER"},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "permissao da tarefa/contrato") {
		t.Fatalf("expected permission error, got %v", err)
	}
}

func TestCallLocalServicePlugin(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s", r.Method)
		}
		if r.Header.Get("X-OBG-Contract-ID") != "11" || r.Header.Get("X-OBG-Task-ID") != "22" {
			t.Fatalf("scope headers missing: %#v", r.Header)
		}
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(raw), "tools/call") || !strings.Contains(string(raw), "echo") {
			t.Fatalf("payload = %s", string(raw))
		}
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	dir := t.TempDir()
	writeManifest(t, dir, Manifest{
		ID:        "svc",
		Name:      "Service",
		Transport: "local-service",
		Endpoint:  server.URL,
		Enabled:   true,
		Sandbox:   SandboxPolicy{MaxOutputBytes: 16000},
		Tools:     []Tool{{Name: "echo", Description: "echo test"}},
	})
	registry, err := LoadRegistry(dir)
	if err != nil {
		t.Fatal(err)
	}
	result, err := registry.Call(context.Background(), CallRequest{
		PluginID:   "svc",
		Tool:       "echo",
		Input:      map[string]any{"value": "ok"},
		ContractID: 11,
		TaskID:     22,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Transport != "local-service" || !result.Sandboxed || result.Output != `{"ok":true}` {
		t.Fatalf("result = %#v", result)
	}
}

func TestCallServicePluginTruncatesOutputAndReportsHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/error" {
			http.Error(w, strings.Repeat("x", 40), http.StatusBadGateway)
			return
		}
		_, _ = w.Write([]byte(strings.Repeat("a", 64)))
	}))
	defer server.Close()

	dir := t.TempDir()
	writeManifest(t, dir, Manifest{
		ID:        "svc",
		Name:      "Service",
		Transport: "local-service",
		Endpoint:  server.URL,
		Enabled:   true,
		Sandbox:   SandboxPolicy{MaxOutputBytes: 12},
		Tools:     []Tool{{Name: "echo", Description: "echo test"}},
	})
	registry, err := LoadRegistry(dir)
	if err != nil {
		t.Fatal(err)
	}
	result, err := registry.Call(context.Background(), CallRequest{PluginID: "svc", Tool: "echo"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.Output, "saida truncada") || len(result.Output) <= 12 {
		t.Fatalf("expected truncated output, got %q", result.Output)
	}

	writeManifest(t, dir, Manifest{
		ID:        "badsvc",
		Name:      "Bad Service",
		Transport: "local-service",
		Endpoint:  server.URL + "/error",
		Enabled:   true,
		Sandbox:   SandboxPolicy{MaxOutputBytes: 12},
		Tools:     []Tool{{Name: "echo", Description: "echo test"}},
	})
	registry, err = LoadRegistry(dir)
	if err != nil {
		t.Fatal(err)
	}
	result, err = registry.Call(context.Background(), CallRequest{PluginID: "badsvc", Tool: "echo"})
	if err == nil || !strings.Contains(err.Error(), "HTTP 502") {
		t.Fatalf("expected HTTP error, got result=%#v err=%v", result, err)
	}
	if !strings.Contains(result.Output, "saida truncada") {
		t.Fatalf("expected truncated error output, got %q", result.Output)
	}
}

func TestCommandRegistrationsClassifyLocalAndWebTargets(t *testing.T) {
	registry := Registry{Plugins: []Plugin{
		{
			Manifest: Manifest{
				ID:        "local",
				Name:      "Local",
				Transport: "stdio-json",
				Command:   "gofmt",
				Args:      []string{"-w"},
				Enabled:   true,
				Tools:     []Tool{{Name: "format"}},
			},
			Path: "plugins/local.json",
		},
		{
			Manifest: Manifest{
				ID:        "svc",
				Name:      "Service",
				Transport: "local-service",
				Endpoint:  "http://127.0.0.1:9000/tool",
				Tools:     []Tool{{Name: "call"}},
			},
			Path: "plugins/svc.json",
		},
		{
			Manifest: Manifest{
				ID:        "script",
				Name:      "Script",
				Transport: "stdio-json",
				Command:   "powershell",
				Args:      []string{"-File", "scripts/audit.ps1"},
				Enabled:   true,
				Sandbox:   SandboxPolicy{ApprovedScripts: []string{"scripts/audit.ps1"}},
				Tools:     []Tool{{Name: "audit"}},
			},
			Path: "plugins/script.json",
		},
		{
			Manifest: Manifest{
				ID:        "blocked",
				Name:      "Blocked",
				Transport: "stdio-json",
				Command:   "gofmt",
				Args:      []string{"-w"},
				Enabled:   false,
				Tools:     []Tool{{Name: "format"}},
			},
			Path: "plugins/blocked.json",
		},
		{
			Manifest: Manifest{
				ID:        "web",
				Name:      "Web",
				Transport: "web-service",
				Endpoint:  "https://example.com/tool",
				Tools:     []Tool{{Name: "call"}},
			},
			Path: "plugins/web.json",
		},
	}}
	commands := registry.CommandRegistrations()
	if len(commands) != 5 {
		t.Fatalf("commands = %#v", commands)
	}
	if commands[0].Kind != "local-command" || commands[0].Status != "enabled" || commands[0].Target != "gofmt -w" {
		t.Fatalf("local command = %#v", commands[0])
	}
	if commands[1].Kind != "local-service" || commands[1].Target != "http://127.0.0.1:9000/tool" {
		t.Fatalf("local service = %#v", commands[1])
	}
	if commands[2].Kind != "local-script" || commands[2].Status != "approved-script" || commands[2].Target != "powershell -File scripts/audit.ps1" {
		t.Fatalf("local script = %#v", commands[2])
	}
	if commands[3].Kind != "local-command" || commands[3].Status != "blocked" {
		t.Fatalf("blocked command = %#v", commands[3])
	}
	if commands[4].Kind != "web-service" || commands[4].Target != "https://example.com/tool" {
		t.Fatalf("web service = %#v", commands[4])
	}
}

func TestLocalServiceRequiresLoopback(t *testing.T) {
	plugin := Plugin{Manifest: Manifest{
		ID:        "svc",
		Name:      "Service",
		Transport: "local-service",
		Endpoint:  "http://192.0.2.10/plugin",
		Enabled:   true,
	}}
	err := plugin.Validate()
	if err == nil || !strings.Contains(err.Error(), "loopback") {
		t.Fatalf("expected loopback validation error, got %v", err)
	}
}

func TestWebServiceRequiresHTTPS(t *testing.T) {
	plugin := Plugin{Manifest: Manifest{
		ID:        "svc",
		Name:      "Service",
		Transport: "web-service",
		Endpoint:  "http://example.com/plugin",
		Enabled:   true,
	}}
	err := plugin.Validate()
	if err == nil || !strings.Contains(err.Error(), "https") {
		t.Fatalf("expected https validation error, got %v", err)
	}
}

func TestWebServiceRejectsLoopbackAndInvalidHeaders(t *testing.T) {
	plugin := Plugin{Manifest: Manifest{
		ID:        "web",
		Name:      "Web",
		Transport: "web-service",
		Endpoint:  "https://localhost/plugin",
		Enabled:   true,
	}}
	err := plugin.Validate()
	if err == nil || !strings.Contains(err.Error(), "localhost") {
		t.Fatalf("expected localhost validation error, got %v", err)
	}
	plugin = Plugin{Manifest: Manifest{
		ID:        "web",
		Name:      "Web",
		Transport: "web-service",
		Endpoint:  "https://example.com/plugin",
		Headers:   map[string]string{"Bad\nHeader": "x"},
		Enabled:   true,
	}}
	err = plugin.Validate()
	if err == nil || !strings.Contains(err.Error(), "header invalido") {
		t.Fatalf("expected header validation error, got %v", err)
	}
	plugin.Manifest.Headers = map[string]string{"X-Test": "bad\r\nvalue"}
	err = plugin.Validate()
	if err == nil || !strings.Contains(err.Error(), "valor de header invalido") {
		t.Fatalf("expected header value validation error, got %v", err)
	}
}

func writeManifest(t *testing.T, dir string, manifest Manifest) {
	t.Helper()
	raw, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, manifest.ID+".json"), raw, 0644); err != nil {
		t.Fatal(err)
	}
}

func commandForTest() string {
	command, err := os.Executable()
	if err != nil {
		panic(err)
	}
	return command
}

func argsForTest() []string {
	return []string{"-test.run=TestPluginHelperProcess", "--"}
}

func envForTest() map[string]string {
	return map[string]string{"OBG_PLUGIN_HELPER": "1"}
}

func sandboxForTest() SandboxPolicy {
	return SandboxPolicy{
		AllowCommands:  []string{commandForTest()},
		AllowEnv:       []string{"OBG_PLUGIN_HELPER"},
		MaxOutputBytes: 16000,
	}
}

func TestPluginHelperProcess(t *testing.T) {
	if os.Getenv("OBG_PLUGIN_HELPER") != "1" {
		return
	}
	_, _ = io.Copy(os.Stdout, os.Stdin)
	os.Exit(0)
}
