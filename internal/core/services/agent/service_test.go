package agent_test

import (
	"context"
	"errors"
	"testing"

	"navi/internal/adapters/persistence/memory"
	"navi/internal/core/domain"
	agentservice "navi/internal/core/services/agent"
)

func newSvc(agents []*domain.Agent) *agentservice.Service {
	return agentservice.New(memory.NewAgentRepository(agents))
}

// ── List ──────────────────────────────────────────────────────────────────────

func TestList_Empty(t *testing.T) {
	svc := newSvc(nil)
	agents, err := svc.List(context.Background())
	if err != nil {
		t.Fatalf("List error: %v", err)
	}
	if len(agents) != 0 {
		t.Errorf("len = %d, want 0", len(agents))
	}
}

func TestList_ReturnsAll(t *testing.T) {
	svc := newSvc([]*domain.Agent{
		{ID: "coder", Name: "Coder"},
		{ID: "researcher", Name: "Researcher"},
	})
	agents, err := svc.List(context.Background())
	if err != nil {
		t.Fatalf("List error: %v", err)
	}
	if len(agents) != 2 {
		t.Errorf("len = %d, want 2", len(agents))
	}
}

// ── Get ───────────────────────────────────────────────────────────────────────

func TestGet_Found(t *testing.T) {
	svc := newSvc([]*domain.Agent{{ID: "coder", Name: "Coder"}})
	a, err := svc.Get(context.Background(), "coder")
	if err != nil {
		t.Fatalf("Get error: %v", err)
	}
	if a.Name != "Coder" {
		t.Errorf("Name = %q, want Coder", a.Name)
	}
}

func TestGet_NotFound_ReturnsErrNotFound(t *testing.T) {
	svc := newSvc(nil)
	_, err := svc.Get(context.Background(), "ghost")
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("error = %v, want ErrNotFound in chain", err)
	}
}
