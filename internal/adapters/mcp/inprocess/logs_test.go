package inprocess

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestNewLogsHandler_BasicFiltersAndLimit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "telemetry.jsonl")
	lines := []string{
		`{"time":"2026-03-03T10:00:00Z","level":"INFO","component":"orchestrator","trace_id":"t-1","msg":"ask_start"}`,
		`{"time":"2026-03-03T10:00:01Z","level":"ERROR","component":"orchestrator","trace_id":"t-2","msg":"ask_failed"}`,
		`{"time":"2026-03-03T10:00:02Z","level":"ERROR","component":"tools","trace_id":"t-2","msg":"tool_failed"}`,
		`{"time":"2026-03-03T10:00:03Z","level":"ERROR","component":"orchestrator","trace_id":"t-2","msg":"ask_failed_again"}`,
	}
	if err := os.WriteFile(path, []byte(lines[0]+"\n"+lines[1]+"\n"+lines[2]+"\n"+lines[3]+"\n"), 0o600); err != nil {
		t.Fatalf("write log file: %v", err)
	}

	h := NewLogsHandler(path)
	out, err := h(context.Background(), `{"limit":1,"level":"error","component":"orchestrator","trace_id":"t-2"}`)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}

	payload := decodeLogsPayload(t, out)
	if got := intValue(payload["matched"]); got != 2 {
		t.Fatalf("matched = %d, want 2", got)
	}
	if got := intValue(payload["returned"]); got != 1 {
		t.Fatalf("returned = %d, want 1", got)
	}

	entries := mapEntries(t, payload["entries"])
	if len(entries) != 1 {
		t.Fatalf("entries len = %d, want 1", len(entries))
	}
	if msg := stringValue(entries[0]["msg"]); msg != "ask_failed_again" {
		t.Fatalf("last entry msg = %q, want ask_failed_again", msg)
	}
}

func TestNewLogsHandler_DefaultAndContains(t *testing.T) {
	path := filepath.Join(t.TempDir(), "telemetry.jsonl")
	if err := os.WriteFile(path, []byte(
		`{"time":"2026-03-03T10:00:00Z","level":"INFO","msg":"boot"}`+"\n"+
			`{"time":"2026-03-03T10:00:01Z","level":"INFO","msg":"tool execution done"}`+"\n"+
			`{"time":"2026-03-03T10:00:02Z","level":"INFO","msg":"shutdown"}`+"\n",
	), 0o600); err != nil {
		t.Fatalf("write log file: %v", err)
	}

	h := NewLogsHandler(path)
	out, err := h(context.Background(), "tool execution")
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}

	payload := decodeLogsPayload(t, out)
	if got := intValue(payload["matched"]); got != 1 {
		t.Fatalf("matched = %d, want 1", got)
	}
	entries := mapEntries(t, payload["entries"])
	if len(entries) != 1 {
		t.Fatalf("entries len = %d, want 1", len(entries))
	}
	if msg := stringValue(entries[0]["msg"]); msg != "tool execution done" {
		t.Fatalf("msg = %q, want tool execution done", msg)
	}
}

func TestNewLogsHandler_FileNotFoundReturnsEmptyResult(t *testing.T) {
	h := NewLogsHandler(filepath.Join(t.TempDir(), "missing.jsonl"))
	out, err := h(context.Background(), "")
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	payload := decodeLogsPayload(t, out)
	if got := intValue(payload["returned"]); got != 0 {
		t.Fatalf("returned = %d, want 0", got)
	}
}

func TestParseLogsQuery_InvalidJSON(t *testing.T) {
	if _, err := parseLogsQuery(`{"limit":`); err == nil {
		t.Fatal("expected error")
	}
}

func decodeLogsPayload(t *testing.T, raw string) map[string]any {
	t.Helper()
	var payload map[string]any
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	return payload
}

func intValue(v any) int {
	f, _ := v.(float64)
	return int(f)
}

func mapEntries(t *testing.T, v any) []map[string]any {
	t.Helper()
	list, ok := v.([]any)
	if !ok {
		t.Fatalf("entries type = %T, want []any", v)
	}
	out := make([]map[string]any, 0, len(list))
	for _, item := range list {
		m, ok := item.(map[string]any)
		if !ok {
			t.Fatalf("entry type = %T, want map[string]any", item)
		}
		out = append(out, m)
	}
	return out
}
