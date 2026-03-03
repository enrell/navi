package inprocess

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
)

const (
	defaultLogsLimit = 20
	maxLogsLimit     = 200
)

type logsQuery struct {
	Limit     int    `json:"limit"`
	Level     string `json:"level"`
	Component string `json:"component"`
	TraceID   string `json:"trace_id"`
	Contains  string `json:"contains"`
}

func NewLogsHandler(logPath string) Handler {
	return func(_ context.Context, input string) (string, error) {
		path := strings.TrimSpace(logPath)
		if path == "" {
			return "", fmt.Errorf("mcp.logs: log path is empty")
		}

		query, err := parseLogsQuery(input)
		if err != nil {
			return "", err
		}

		f, err := os.Open(path)
		if err != nil {
			if os.IsNotExist(err) {
				payload := map[string]any{
					"log_path": path,
					"matched":  0,
					"returned": 0,
					"entries":  []map[string]any{},
				}
				b, _ := json.Marshal(payload)
				return string(b), nil
			}
			return "", fmt.Errorf("mcp.logs: open log file: %w", err)
		}
		defer f.Close()

		scanner := bufio.NewScanner(f)
		scanner.Buffer(make([]byte, 1024), 1024*1024)

		entries := make([]map[string]any, 0, query.Limit)
		matched := 0
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" {
				continue
			}

			var event map[string]any
			if err := json.Unmarshal([]byte(line), &event); err != nil {
				continue
			}

			if !matchLogsQuery(event, query, line) {
				continue
			}

			matched++
			entries = append(entries, event)
			if len(entries) > query.Limit {
				entries = entries[1:]
			}
		}

		if err := scanner.Err(); err != nil {
			return "", fmt.Errorf("mcp.logs: scan log file: %w", err)
		}

		payload := map[string]any{
			"log_path": path,
			"matched":  matched,
			"returned": len(entries),
			"entries":  entries,
		}
		out, err := json.Marshal(payload)
		if err != nil {
			return "", fmt.Errorf("mcp.logs: marshal response: %w", err)
		}
		return string(out), nil
	}
}

func parseLogsQuery(raw string) (logsQuery, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return logsQuery{Limit: defaultLogsLimit}, nil
	}

	if n, err := strconv.Atoi(raw); err == nil {
		return logsQuery{Limit: clampLogsLimit(n)}, nil
	}

	if !strings.HasPrefix(raw, "{") {
		return logsQuery{Limit: defaultLogsLimit, Contains: strings.ToLower(raw)}, nil
	}

	var q logsQuery
	if err := json.Unmarshal([]byte(raw), &q); err != nil {
		return logsQuery{}, fmt.Errorf("mcp.logs: invalid JSON input: %w", err)
	}

	q.Level = strings.ToLower(strings.TrimSpace(q.Level))
	q.Component = strings.TrimSpace(q.Component)
	q.TraceID = strings.TrimSpace(q.TraceID)
	q.Contains = strings.ToLower(strings.TrimSpace(q.Contains))
	q.Limit = clampLogsLimit(q.Limit)
	return q, nil
}

func clampLogsLimit(v int) int {
	if v <= 0 {
		return defaultLogsLimit
	}
	if v > maxLogsLimit {
		return maxLogsLimit
	}
	return v
}

func matchLogsQuery(event map[string]any, query logsQuery, rawLine string) bool {
	if query.Level != "" {
		if strings.ToLower(strings.TrimSpace(stringValue(event["level"]))) != query.Level {
			return false
		}
	}

	if query.Component != "" {
		if strings.TrimSpace(stringValue(event["component"])) != query.Component {
			return false
		}
	}

	if query.TraceID != "" {
		if strings.TrimSpace(stringValue(event["trace_id"])) != query.TraceID {
			return false
		}
	}

	if query.Contains != "" {
		if !strings.Contains(strings.ToLower(rawLine), query.Contains) {
			return false
		}
	}

	return true
}

func stringValue(v any) string {
	switch tv := v.(type) {
	case string:
		return tv
	default:
		return ""
	}
}
