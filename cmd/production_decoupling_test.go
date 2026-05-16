package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProductionFlowHasNoModelRuntimeDependencies(t *testing.T) {
	root := repoRoot(t)
	targets := []string{"cmd", "database", "engine", "knowledge", "mcp", "scripts", "web"}
	banned := []string{
		"GGUF",
		"gguf",
		"tokenizer",
		"tensor",
		"transformer",
		"-model",
		"models",
	}
	for _, target := range targets {
		base := filepath.Join(root, target)
		err := filepath.WalkDir(base, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
				return nil
			}
			if strings.HasSuffix(path, "_test.go") || strings.HasSuffix(path, ".exe") {
				return nil
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			text := string(raw)
			for _, term := range banned {
				if strings.Contains(text, term) {
					t.Fatalf("production file %s still references removed model dependency %q", path, term)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod nao encontrado")
		}
		dir = parent
	}
}
