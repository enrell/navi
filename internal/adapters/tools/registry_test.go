package tools_test

import (
	"context"
	"strings"
	"testing"

	"navi/internal/adapters/tools"
)

func TestRegistry_RegisterListExecute(t *testing.T) {
	r := tools.NewRegistry()
	if err := r.Register("native.echo", "Echo", func(_ context.Context, input string) (string, error) {
		return strings.TrimSpace(input), nil
	}); err != nil {
		t.Fatalf("Register error: %v", err)
	}

	list, err := r.ListTools(context.Background())
	if err != nil {
		t.Fatalf("ListTools error: %v", err)
	}
	if len(list) != 1 || list[0].Name != "native.echo" {
		t.Fatalf("unexpected list: %+v", list)
	}

	got, err := r.ExecuteTool(context.Background(), "native.echo", "  hello  ")
	if err != nil {
		t.Fatalf("ExecuteTool error: %v", err)
	}
	if got != "hello" {
		t.Errorf("got %q, want hello", got)
	}
}

func TestRegistry_UnknownToolError(t *testing.T) {
	r := tools.NewRegistry()
	_, err := r.ExecuteTool(context.Background(), "missing", "")
	if err == nil {
		t.Fatal("expected error")
	}
}
