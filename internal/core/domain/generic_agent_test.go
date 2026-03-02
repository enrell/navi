package domain_test

import (
	"testing"

	"navi/internal/core/domain"
)

func TestNewGenericAgent_Defaults(t *testing.T) {
	a, err := domain.NewGenericAgent(domain.AgentConfig{ID: "coder"}, "You are coder")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if a.Type() != domain.AgentTypeGeneric {
		t.Errorf("type = %q, want generic", a.Type())
	}
	if a.Config().Name != "coder" {
		t.Errorf("name = %q, want coder", a.Config().Name)
	}
	if a.Config().Status != domain.AgentStatusTrusted {
		t.Errorf("status = %q, want trusted", a.Config().Status)
	}
}

func TestNewGenericAgent_UnsupportedType(t *testing.T) {
	_, err := domain.NewGenericAgent(domain.AgentConfig{ID: "x", Type: domain.AgentType("custom")}, "prompt")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestGenericAgent_AsAgentAndBuildMessages(t *testing.T) {
	a, err := domain.NewGenericAgent(domain.AgentConfig{
		ID:           "researcher",
		Type:         domain.AgentTypeGeneric,
		Name:         "Researcher",
		Description:  "Finds information",
		Capabilities: []string{"network:api.github.com:443"},
		Status:       domain.AgentStatusModified,
	}, "System Prompt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	meta := a.AsAgent()
	if meta.ID != "researcher" || meta.Type != "generic" {
		t.Fatalf("unexpected metadata: %+v", meta)
	}

	msgs := a.BuildMessages("hello")
	if len(msgs) != 2 {
		t.Fatalf("len = %d, want 2", len(msgs))
	}
	if msgs[0].Role != domain.RoleSystem || msgs[1].Role != domain.RoleUser {
		t.Fatalf("unexpected roles: %+v", msgs)
	}
}
