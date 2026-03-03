package telemetry_test

import (
	"context"
	"path/filepath"
	"testing"

	"navi/internal/telemetry"
)

func TestTraceIDHelpers(t *testing.T) {
	ctx, traceID := telemetry.EnsureTraceID(context.Background())
	if traceID == "" {
		t.Fatal("expected trace id")
	}
	if got := telemetry.TraceID(ctx); got != traceID {
		t.Fatalf("TraceID = %q, want %q", got, traceID)
	}

	ctx2 := telemetry.WithTraceID(context.Background(), "fixed-trace")
	if got := telemetry.TraceID(ctx2); got != "fixed-trace" {
		t.Fatalf("TraceID = %q, want fixed-trace", got)
	}
}

func TestLogPath_UsesOverride(t *testing.T) {
	override := filepath.Join(t.TempDir(), "telemetry.jsonl")
	t.Setenv("NAVI_LOG_PATH", override)

	path, err := telemetry.LogPath()
	if err != nil {
		t.Fatalf("LogPath error: %v", err)
	}
	if path != override {
		t.Fatalf("LogPath = %q, want %q", path, override)
	}
}
