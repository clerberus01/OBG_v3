package sandbox

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBlocksDangerousCommandsEvenWhenAllowlisted(t *testing.T) {
	cases := []struct {
		command string
		args    []string
	}{
		{command: "rm", args: []string{"-rf", "."}},
		{command: "del", args: []string{"/f", "file.txt"}},
		{command: "format", args: []string{"C:"}},
		{command: "shutdown", args: []string{"/s"}},
		{command: "cmd", args: []string{"/c", "echo ok"}},
		{command: "bash", args: []string{"-lc", "echo ok"}},
		{command: "taskkill", args: []string{"/PID", "1"}},
	}
	for _, tc := range cases {
		t.Run(tc.command, func(t *testing.T) {
			if CommandAllowed(tc.command, tc.args, Policy{AllowCommands: []string{tc.command}}, ".") {
				t.Fatalf("expected dangerous command to be blocked: %s", tc.command)
			}
		})
	}
}

func TestAllowsOnlyApprovedGoOperations(t *testing.T) {
	if !CommandAllowed("go", []string{"test", "./..."}, Policy{}, ".") {
		t.Fatal("expected go test to be allowed")
	}
	if !CommandAllowed("go", []string{"build", "./cmd"}, Policy{}, ".") {
		t.Fatal("expected go build to be allowed")
	}
	if CommandAllowed("go", []string{"run", "./cmd"}, Policy{}, ".") {
		t.Fatal("expected go run to be blocked")
	}
}

func TestAllowsGofmt(t *testing.T) {
	if !CommandAllowed("gofmt", []string{"-w", "mcp/registry.go"}, Policy{}, ".") {
		t.Fatal("expected gofmt to be allowed")
	}
}

func TestAllowsOnlyApprovedLocalScripts(t *testing.T) {
	root := t.TempDir()
	policy := Policy{ApprovedScripts: []string{"scripts/test.ps1"}}
	if !CommandAllowed("powershell", []string{"-NoProfile", "-File", "scripts/test.ps1"}, policy, root) {
		t.Fatal("expected approved local script to be allowed")
	}
	if CommandAllowed("powershell", []string{"-NoProfile", "-Command", "scripts/test.ps1"}, policy, root) {
		t.Fatal("expected powershell -Command to be blocked")
	}
	if CommandAllowed("powershell", []string{"-NoProfile", "-File", "scripts/test.ps1", "-EncodedCommand", "AAAA"}, policy, root) {
		t.Fatal("expected powershell -EncodedCommand to be blocked even after -File")
	}
	if CommandAllowed("powershell", []string{"-NoProfile", "-File", "scripts/other.ps1"}, policy, root) {
		t.Fatal("expected unapproved local script to be blocked")
	}
}

func TestControlledGitSupport(t *testing.T) {
	policy := Policy{AllowCommands: []string{"git status", "git diff", "git remote -v"}}
	if !CommandAllowed("git", []string{"status", "--short"}, policy, ".") {
		t.Fatal("expected git status to be allowed when explicitly registered")
	}
	if !CommandAllowed("git", []string{"remote", "-v"}, policy, ".") {
		t.Fatal("expected git remote -v to be allowed when explicitly registered")
	}
	if CommandAllowed("git", []string{"remote", "-v", "origin"}, policy, ".") {
		t.Fatal("expected git remote -v with extra args to be blocked")
	}
	if CommandAllowed("git", []string{"checkout", "main"}, Policy{AllowCommands: []string{"git checkout"}}, ".") {
		t.Fatal("expected mutating git checkout to be blocked")
	}
	if CommandAllowed("git", []string{"status"}, Policy{AllowCommands: []string{"git"}}, ".") {
		t.Fatal("expected bare git allowlist entry to be insufficient")
	}
}

func TestMutableGitCommandsRemainBlockedForV200(t *testing.T) {
	cases := []struct {
		name    string
		args    []string
		allowed []string
	}{
		{name: "checkout", args: []string{"checkout", "main"}, allowed: []string{"git checkout"}},
		{name: "commit", args: []string{"commit", "-m", "release"}, allowed: []string{"git commit"}},
		{name: "push", args: []string{"push", "origin", "main"}, allowed: []string{"git push"}},
		{name: "pull", args: []string{"pull"}, allowed: []string{"git pull"}},
		{name: "reset", args: []string{"reset", "--hard"}, allowed: []string{"git reset"}},
		{name: "clean", args: []string{"clean", "-fd"}, allowed: []string{"git clean"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if CommandAllowed("git", tc.args, Policy{AllowCommands: tc.allowed}, ".") {
				t.Fatalf("expected mutable git command to remain blocked: git %s", strings.Join(tc.args, " "))
			}
		})
	}
}

func TestControlledDockerPodmanSupport(t *testing.T) {
	policy := Policy{AllowCommands: []string{"docker ps", "docker compose ps", "podman logs"}}
	if !CommandAllowed("docker", []string{"ps", "--all"}, policy, ".") {
		t.Fatal("expected docker ps to be allowed")
	}
	if !CommandAllowed("docker", []string{"compose", "ps"}, policy, ".") {
		t.Fatal("expected docker compose ps to be allowed")
	}
	if !CommandAllowed("podman", []string{"logs", "service"}, policy, ".") {
		t.Fatal("expected podman logs to be allowed")
	}
	if CommandAllowed("docker", []string{"run", "alpine"}, Policy{AllowCommands: []string{"docker run"}}, ".") {
		t.Fatal("expected docker run to be blocked")
	}
	if CommandAllowed("docker", []string{"ps", "--host", "tcp://example:2375"}, Policy{AllowCommands: []string{"docker ps"}}, ".") {
		t.Fatal("expected remote docker host flag to be blocked")
	}
}

func TestMutableContainerCommandsRemainBlockedForV200(t *testing.T) {
	cases := []struct {
		command string
		args    []string
		allowed []string
	}{
		{command: "docker", args: []string{"run", "alpine"}, allowed: []string{"docker run"}},
		{command: "docker", args: []string{"build", "."}, allowed: []string{"docker build"}},
		{command: "docker", args: []string{"compose", "up"}, allowed: []string{"docker compose up"}},
		{command: "docker", args: []string{"compose", "down"}, allowed: []string{"docker compose down"}},
		{command: "podman", args: []string{"run", "alpine"}, allowed: []string{"podman run"}},
		{command: "podman", args: []string{"build", "."}, allowed: []string{"podman build"}},
	}
	for _, tc := range cases {
		t.Run(tc.command+"_"+strings.Join(tc.args, "_"), func(t *testing.T) {
			if CommandAllowed(tc.command, tc.args, Policy{AllowCommands: tc.allowed}, ".") {
				t.Fatalf("expected mutable container command to remain blocked: %s %s", tc.command, strings.Join(tc.args, " "))
			}
		})
	}
}

func TestControlledCurlSupport(t *testing.T) {
	policy := Policy{AllowCommands: []string{"curl https://example.com/health", "curl -I https://example.com/health"}}
	if !CommandAllowed("curl", []string{"https://example.com/health"}, policy, ".") {
		t.Fatal("expected curl GET to be allowed")
	}
	if !CommandAllowed("curl", []string{"-I", "https://example.com/health"}, policy, ".") {
		t.Fatal("expected curl HEAD to be allowed")
	}
	if CommandAllowed("curl", []string{"-X", "POST", "https://example.com/health"}, Policy{AllowCommands: []string{"curl -X POST https://example.com/health"}}, ".") {
		t.Fatal("expected curl POST to be blocked")
	}
	if CommandAllowed("curl", []string{"-o", "file.txt", "https://example.com/health"}, Policy{AllowCommands: []string{"curl -o file.txt https://example.com/health"}}, ".") {
		t.Fatal("expected curl output write to be blocked")
	}
	if CommandAllowed("curl", []string{"--output=file.txt", "https://example.com/health"}, Policy{AllowCommands: []string{"curl --output=file.txt https://example.com/health"}}, ".") {
		t.Fatal("expected curl --output=file to be blocked")
	}
	if CommandAllowed("curl", []string{"--data=payload", "https://example.com/health"}, Policy{AllowCommands: []string{"curl --data=payload https://example.com/health"}}, ".") {
		t.Fatal("expected curl --data=payload to be blocked")
	}
	if CommandAllowed("curl", []string{"-H", "Authorization: token", "https://example.com/health"}, Policy{AllowCommands: []string{"curl -H Authorization: token https://example.com/health"}}, ".") {
		t.Fatal("expected curl custom header to be blocked")
	}
	if CommandAllowed("curl", []string{"--request=POST", "https://example.com/health"}, Policy{AllowCommands: []string{"curl --request=POST https://example.com/health"}}, ".") {
		t.Fatal("expected curl --request=POST to be blocked")
	}
}

func TestPrepareBlocksUndeclaredEnv(t *testing.T) {
	_, err := Prepare(Request{
		PluginID:     "p",
		ManifestPath: filepath.Join(t.TempDir(), "p.json"),
		Command:      "gofmt",
		Env:          map[string]string{"SECRET_TOKEN": "nope"},
		Policy:       Policy{},
	})
	if err == nil || err.Error() != "sandbox bloqueou env SECRET_TOKEN" {
		t.Fatalf("expected env error, got %v", err)
	}
}

func TestPrepareCreatesConfinedWorkDir(t *testing.T) {
	dir := t.TempDir()
	exec, err := Prepare(Request{
		PluginID:     "p",
		ManifestPath: filepath.Join(dir, "p.json"),
		Command:      "gofmt",
		Policy:       Policy{WorkDir: "sub"},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(dir, ".sandbox", "p", "sub")
	if exec.WorkDir != want {
		t.Fatalf("workdir = %q want %q", exec.WorkDir, want)
	}
}

func TestPrepareScopesWorkDirByContractAndTask(t *testing.T) {
	dir := t.TempDir()
	exec, err := Prepare(Request{
		PluginID:     "p",
		ManifestPath: filepath.Join(dir, "p.json"),
		Command:      "gofmt",
		Policy:       Policy{WorkDir: "base"},
		ContractID:   12,
		TaskID:       34,
	})
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(dir, ".sandbox", "p", "base", "contract-12", "task-34")
	if exec.WorkDir != want {
		t.Fatalf("workdir = %q want %q", exec.WorkDir, want)
	}
}

func TestPreparePermissionNarrowsCommand(t *testing.T) {
	_, err := Prepare(Request{
		PluginID:     "p",
		ManifestPath: filepath.Join(t.TempDir(), "p.json"),
		Command:      "gofmt",
		Policy:       Policy{},
		Permissions:  Policy{AllowCommands: []string{"go test"}},
	})
	if err == nil || !strings.Contains(err.Error(), "permissao da tarefa/contrato") {
		t.Fatalf("expected permission command error, got %v", err)
	}
}

func TestPreparePermissionNarrowsEnv(t *testing.T) {
	_, err := Prepare(Request{
		PluginID:     "p",
		ManifestPath: filepath.Join(t.TempDir(), "p.json"),
		Command:      "gofmt",
		Env:          map[string]string{"TOKEN": "x"},
		Policy:       Policy{AllowEnv: []string{"TOKEN"}},
		Permissions:  Policy{AllowEnv: []string{"OTHER"}},
	})
	if err == nil || err.Error() != "sandbox bloqueou env TOKEN" {
		t.Fatalf("expected env permission error, got %v", err)
	}
}

func TestServiceEndpointValidation(t *testing.T) {
	if !SupportedTransport("local-service") || !SupportedTransport("web-service") || SupportedTransport("ssh") {
		t.Fatal("unexpected supported transport result")
	}
	if err := ValidateServiceEndpoint("local-service", "http://127.0.0.1:9000/tool"); err != nil {
		t.Fatal(err)
	}
	if err := ValidateServiceEndpoint("local-service", "http://192.0.2.10/tool"); err == nil || !strings.Contains(err.Error(), "loopback") {
		t.Fatalf("expected loopback error, got %v", err)
	}
	if err := ValidateServiceEndpoint("web-service", "https://example.com/tool"); err != nil {
		t.Fatal(err)
	}
	if err := ValidateServiceEndpoint("web-service", "https://127.0.0.1/tool"); err == nil || !strings.Contains(err.Error(), "localhost") {
		t.Fatalf("expected web-service loopback error, got %v", err)
	}
	if err := ValidateServiceEndpoint("web-service", "http://example.com/tool"); err == nil || !strings.Contains(err.Error(), "https") {
		t.Fatalf("expected https error, got %v", err)
	}
}

func TestHeaderValidation(t *testing.T) {
	if err := ValidateHeaders(map[string]string{"X-OBG-Test": "ok"}); err != nil {
		t.Fatal(err)
	}
	if err := ValidateHeaders(map[string]string{"Bad\nHeader": "ok"}); err == nil {
		t.Fatal("expected bad header name to be rejected")
	}
	if err := ValidateHeaders(map[string]string{"X-Test": "bad\r\nvalue"}); err == nil {
		t.Fatal("expected bad header value to be rejected")
	}
}

func TestRunExecutesInsideSandbox(t *testing.T) {
	dir := t.TempDir()
	result, err := Run(context.Background(), Request{
		PluginID:     "p",
		ManifestPath: filepath.Join(dir, "p.json"),
		Command:      commandForTest(),
		Args:         argsForTest(),
		Env:          map[string]string{"OBG_SANDBOX_HELPER": "1"},
		Policy: Policy{
			AllowCommands:  []string{commandForTest()},
			AllowEnv:       []string{"OBG_SANDBOX_HELPER"},
			MaxOutputBytes: 16000,
		},
	}, `{"ok":true}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.Output, `"ok":true`) {
		t.Fatalf("output = %q", result.Output)
	}
	if result.WorkDir == "" || !strings.Contains(result.WorkDir, filepath.Join(".sandbox", "p")) {
		t.Fatalf("workdir = %q", result.WorkDir)
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
	return []string{"-test.run=TestSandboxHelperProcess", "--"}
}

func TestSandboxHelperProcess(t *testing.T) {
	if os.Getenv("OBG_SANDBOX_HELPER") != "1" {
		return
	}
	_, _ = io.Copy(os.Stdout, os.Stdin)
	os.Exit(0)
}
