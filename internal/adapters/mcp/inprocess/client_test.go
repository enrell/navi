package inprocess_test

import (
	"context"
	"testing"

	"navi/internal/adapters/mcp/inprocess"
)

func TestClient_RegisterListCall(t *testing.T) {
	c := inprocess.New()
	if err := c.Register("echo", func(_ context.Context, input string) (string, error) {
		return "mcp:" + input, nil
	}); err != nil {
		t.Fatalf("Register error: %v", err)
	}

	list, err := c.ListTools(context.Background())
	if err != nil {
		t.Fatalf("ListTools error: %v", err)
	}
	if len(list) != 1 || list[0] != "echo" {
		t.Fatalf("unexpected list: %+v", list)
	}

	got, err := c.CallTool(context.Background(), "echo", "hi")
	if err != nil {
		t.Fatalf("CallTool error: %v", err)
	}
	if got != "mcp:hi" {
		t.Errorf("got %q, want mcp:hi", got)
	}
}
