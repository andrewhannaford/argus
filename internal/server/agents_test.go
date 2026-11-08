package server

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAgentAllowed(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "agents.json")
	if err := os.WriteFile(path, []byte(`{"dc01":"tokenA","web":"tokenB"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	s := &Server{agentsFile: path, agentToken: "legacy"}

	if !s.agentAllowed("dc01", "tokenA") {
		t.Fatal("per-agent token should match its agent id")
	}
	if s.agentAllowed("dc01", "wrong") {
		t.Fatal("wrong token should not match")
	}
	if s.agentAllowed("web", "tokenA") {
		t.Fatal("token is bound to its id; tokenA must not match web")
	}
	if !s.agentAllowed("unknown", "legacy") {
		t.Fatal("legacy token should match an id without a per-agent token")
	}
	if s.agentAllowed("unknown", "nope") {
		t.Fatal("wrong legacy token should not match")
	}
	if s.agentAllowed("dc01", "legacy") {
		t.Fatal("legacy token must not match an id that has a per-agent token")
	}
}

func TestAgentAllowedNoLegacy(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "agents.json")
	if err := os.WriteFile(path, []byte(`{"dc01":"tokenA"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	s := &Server{agentsFile: path, agentToken: ""}

	if !s.agentAllowed("dc01", "tokenA") {
		t.Fatal("per-agent token should match")
	}
	if s.agentAllowed("unknown", "tokenA") {
		t.Fatal("no legacy token: unknown id must be rejected")
	}
}
