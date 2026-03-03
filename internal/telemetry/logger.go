package telemetry

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"sync"

	"github.com/google/uuid"
)

var (
	mu            sync.RWMutex
	global        = slog.New(slog.NewJSONHandler(io.Discard, nil))
	traceContextK = struct{}{}
)

func Logger() *slog.Logger {
	mu.RLock()
	defer mu.RUnlock()
	return global
}

func SetLogger(logger *slog.Logger) {
	if logger == nil {
		return
	}
	mu.Lock()
	global = logger
	mu.Unlock()
}

func LogPath() (string, error) {
	if override := os.Getenv("NAVI_LOG_PATH"); override != "" {
		return override, nil
	}

	if state := os.Getenv("XDG_STATE_HOME"); state != "" {
		return filepath.Join(state, "navi", "logs", "telemetry.jsonl"), nil
	}

	if runtime.GOOS == "windows" {
		if local := os.Getenv("LOCALAPPDATA"); local != "" {
			return filepath.Join(local, "navi", "logs", "telemetry.jsonl"), nil
		}
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("telemetry: user home: %w", err)
	}
	return filepath.Join(home, ".local", "state", "navi", "logs", "telemetry.jsonl"), nil
}

func InitDefaultJSONLLogger() (func() error, error) {
	path, err := LogPath()
	if err != nil {
		return nil, err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("telemetry: mkdir logs dir: %w", err)
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, fmt.Errorf("telemetry: open log file: %w", err)
	}

	h := slog.NewJSONHandler(f, &slog.HandlerOptions{Level: slog.LevelDebug})
	SetLogger(slog.New(h).With("app", "navi"))

	Logger().Info("telemetry_initialized", "path", path)
	return f.Close, nil
}

func EnsureTraceID(ctx context.Context) (context.Context, string) {
	if ctx == nil {
		ctx = context.Background()
	}
	if id, ok := ctx.Value(traceContextK).(string); ok && id != "" {
		return ctx, id
	}
	id := uuid.NewString()
	return context.WithValue(ctx, traceContextK, id), id
}

func WithTraceID(ctx context.Context, traceID string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if traceID == "" {
		return ctx
	}
	return context.WithValue(ctx, traceContextK, traceID)
}

func TraceID(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	id, _ := ctx.Value(traceContextK).(string)
	return id
}
