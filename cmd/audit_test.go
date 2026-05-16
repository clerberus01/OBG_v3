package main

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"omni-bot-go/database"
)

func TestAuditRejectsNarrativeNoise(t *testing.T) {
	contract := auditFixtureContract()
	payload := auditFixturePayload(t, contract)
	payload["report"] = "Como uma IA, espero que isso ajude. " + payload["report"].(string)
	if err := audit(contract, payload); err == nil || !strings.Contains(err.Error(), "lero-lero") {
		t.Fatalf("expected narrative noise rejection, got %v", err)
	}
}

func TestAuditRejectsLooping(t *testing.T) {
	contract := auditFixtureContract()
	payload := auditFixturePayload(t, contract)
	payload["report"] = strings.Repeat("contrato modulo tecnico validado ", 5)
	if err := audit(contract, payload); err == nil || !strings.Contains(err.Error(), "loop") {
		t.Fatalf("expected loop rejection, got %v", err)
	}
}

func TestAuditRejectsContractHashMismatch(t *testing.T) {
	contract := auditFixtureContract()
	payload := auditFixturePayload(t, contract)
	payload["contract_hash"] = "hash-errado"
	if err := audit(contract, payload); err == nil || !strings.Contains(err.Error(), "hash") {
		t.Fatalf("expected hash rejection, got %v", err)
	}
}

func TestAuditAcceptsContractBoundOutput(t *testing.T) {
	contract := auditFixtureContract()
	payload := auditFixturePayload(t, contract)
	if err := audit(contract, payload); err != nil {
		t.Fatal(err)
	}
}

func auditFixtureContract() database.Contract {
	return database.Contract{
		ID:           1,
		NorthStar:    "Construir modulo tecnico auditavel",
		Constraints:  "Contrato imutavel por hash",
		Deliverables: "Artefato selado com relatorio tecnico",
		Hash:         "hash-ok",
		Sealed:       true,
	}
}

func auditFixturePayload(t *testing.T, contract database.Contract) map[string]any {
	t.Helper()
	path := filepath.Join(t.TempDir(), "artifact.json")
	data := []byte(`{"ok":true}`)
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}
	artifact := fileArtifact{
		Path:   path,
		SHA256: sha256Hex(data),
		Bytes:  len(data),
	}
	return map[string]any{
		"task_id":       int64(1),
		"role":          "Auditor Tecnico",
		"contract_id":   contract.ID,
		"contract_hash": contract.Hash,
		"report":        "Modulo tecnico auditavel entregue com artefato selado e relatorio tecnico.",
		"artifacts":     []fileArtifact{artifact},
	}
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
