package domain

import (
	"testing"
)

func TestCapability_Raw_ThreeParts(t *testing.T) {
	c := Capability{Type: "filesystem", Resource: "workspace", Mode: "rw"}
	got := c.Raw()
	want := "filesystem:workspace:rw"
	if got != want {
		t.Errorf("Raw() = %q, want %q", got, want)
	}
}

func TestCapability_Raw_TwoParts(t *testing.T) {
	c := Capability{Type: "exec", Resource: "bash,go,git"}
	got := c.Raw()
	want := "exec:bash,go,git"
	if got != want {
		t.Errorf("Raw() = %q, want %q", got, want)
	}
}

func TestCapability_Raw_OnePart(t *testing.T) {
	c := Capability{Type: "vision"}
	got := c.Raw()
	want := "vision"
	if got != want {
		t.Errorf("Raw() = %q, want %q", got, want)
	}
}

func TestCapability_Raw_EmptyResourceNonEmptyMode(t *testing.T) {
	c := Capability{Type: "test", Mode: "rw"}
	got := c.Raw()
	want := "test:rw"
	if got != want {
		t.Errorf("Raw() = %q, want %q", got, want)
	}
}

func TestParseCapability_ThreeParts(t *testing.T) {
	c, err := ParseCapability("filesystem:workspace:rw")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.Type != "filesystem" || c.Resource != "workspace" || c.Mode != "rw" {
		t.Errorf("got {%s %s %s}, want {filesystem workspace rw}", c.Type, c.Resource, c.Mode)
	}
}

func TestParseCapability_TwoParts(t *testing.T) {
	c, err := ParseCapability("exec:bash,go")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.Type != "exec" || c.Resource != "bash,go" || c.Mode != "" {
		t.Errorf("got {%s %s %s}, want {exec bash,go }", c.Type, c.Resource, c.Mode)
	}
}

func TestParseCapability_OnePart(t *testing.T) {
	c, err := ParseCapability("vision")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.Type != "vision" || c.Resource != "" || c.Mode != "" {
		t.Errorf("got {%s %s %s}, want {vision  }", c.Type, c.Resource, c.Mode)
	}
}

func TestParseCapability_NetworkWithPort(t *testing.T) {
	c, err := ParseCapability("network:api.github.com:443")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.Type != "network" || c.Resource != "api.github.com" || c.Mode != "443" {
		t.Errorf("got {%s %s %s}, want {network api.github.com 443}", c.Type, c.Resource, c.Mode)
	}
}

func TestParseCapability_EmptyString(t *testing.T) {
	_, err := ParseCapability("")
	if err == nil {
		t.Error("expected error for empty string, got nil")
	}
}

func TestParseCapability_ColonOnly(t *testing.T) {
	_, err := ParseCapability(":")
	if err == nil {
		t.Error("expected error for ':' input, got nil")
	}
}

func TestParseCapability_Roundtrip(t *testing.T) {
	cases := []string{
		"filesystem:workspace:rw",
		"exec:bash,go",
		"vision",
		"network:*:443",
		"tool:mcp-ast",
		"ocr:tesseract",
	}
	for _, raw := range cases {
		c, err := ParseCapability(raw)
		if err != nil {
			t.Fatalf("ParseCapability(%q) error: %v", raw, err)
		}
		got := c.Raw()
		if got != raw {
			t.Errorf("roundtrip failed: %q -> Raw() = %q", raw, got)
		}
	}
}
