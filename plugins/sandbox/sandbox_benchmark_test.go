package sandbox

import (
	"path/filepath"
	"testing"
)

func BenchmarkCommandAllowedControlledGit(b *testing.B) {
	policy := Policy{AllowCommands: []string{"git status", "git diff", "git remote -v"}}
	args := []string{"status", "--short"}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if !CommandAllowed("git", args, policy, ".") {
			b.Fatal("git status should be allowed")
		}
	}
}

func BenchmarkPrepareScopedRequest(b *testing.B) {
	root := b.TempDir()
	request := Request{
		PluginID:     "bench",
		ManifestPath: filepath.Join(root, "bench.json"),
		Command:      "gofmt",
		Args:         []string{"-w", "mcp/registry.go"},
		Policy: Policy{
			WorkDir:       "work",
			AllowCommands: []string{"gofmt"},
		},
		ContractID: 10,
		TaskID:     20,
	}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := Prepare(request); err != nil {
			b.Fatal(err)
		}
	}
}
