package capabilities_test

import (
	"testing"

	"navi/internal/core/domain"
	"navi/internal/core/services/capabilities"
)

func TestParse(t *testing.T) {
	tests := []struct {
		raw                              string
		wantType, wantResource, wantMode string
	}{
		{"filesystem:workspace:rw", "filesystem", "workspace", "rw"},
		{"exec:bash,go,git", "exec", "bash,go,git", ""},
		{"network:api.github.com:443", "network", "api.github.com", "443"},
		{"tool:mcp-ast", "tool", "mcp-ast", ""},
		{"vision", "vision", "", ""},
		{"ocr:tesseract", "ocr", "tesseract", ""},
	}
	for _, tt := range tests {
		caps, err := capabilities.Parse([]string{tt.raw})
		if err != nil {
			t.Fatalf("Parse(%q) error: %v", tt.raw, err)
		}
		if len(caps) != 1 {
			t.Fatalf("expected 1 cap, got %d", len(caps))
		}
		c := caps[0]
		if c.Type != tt.wantType || c.Resource != tt.wantResource || c.Mode != tt.wantMode {
			t.Errorf("Parse(%q) = {%s %s %s}, want {%s %s %s}",
				tt.raw, c.Type, c.Resource, c.Mode,
				tt.wantType, tt.wantResource, tt.wantMode)
		}
	}
}

func TestSatisfies(t *testing.T) {
	agentCaps := []domain.Capability{
		{Type: "filesystem", Resource: "workspace", Mode: "rw"},
		{Type: "exec", Resource: "bash,go"},
		{Type: "network", Resource: "api.github.com", Mode: "443"},
	}
	required := []domain.Capability{
		{Type: "filesystem", Resource: "workspace"},
		{Type: "exec", Resource: "bash,go"},
	}
	if !capabilities.Satisfies(agentCaps, required) {
		t.Error("expected Satisfies to return true")
	}
	missing := []domain.Capability{
		{Type: "vision"},
	}
	if capabilities.Satisfies(agentCaps, missing) {
		t.Error("expected Satisfies to return false for missing cap")
	}
}
