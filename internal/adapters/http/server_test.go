package httpserver_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	httpserver "navi/internal/adapters/http"
	"navi/internal/adapters/persistence/memory"
	"navi/internal/core/domain"
	"navi/internal/core/ports"
	agentsvc "navi/internal/core/services/agent"
	chatservice "navi/internal/core/services/chat"
	tasksvc "navi/internal/core/services/task"
)

// ── test doubles ─────────────────────────────────────────────────────────────

type stubLLM struct {
	reply string
	err   error
}

func (s *stubLLM) Chat(_ context.Context, _ []domain.Message) (string, error) {
	return s.reply, s.err
}

var _ ports.LLMPort = (*stubLLM)(nil)

// newServer wires up a complete Server with an in-memory store and a stub LLM.
func newServer(llm ports.LLMPort, agents []*domain.Agent) *httpserver.Server {
	taskRepo := memory.NewTaskRepository()
	agentRepo := memory.NewAgentRepository(agents)

	chat := chatservice.New(llm)
	tasks := tasksvc.New(taskRepo, chat)
	agentService := agentsvc.New(agentRepo)

	return httpserver.New(tasks, agentService)
}

// do is a convenience helper to perform a request against the server.
func do(t *testing.T, srv *httpserver.Server, method, path string, body any) *http.Response {
	t.Helper()
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)

	var bodyReader *bytes.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		bodyReader = bytes.NewReader(b)
	} else {
		bodyReader = bytes.NewReader(nil)
	}

	req, err := http.NewRequestWithContext(context.Background(), method, ts.URL+path, bodyReader)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	return resp
}

// decodeJSON decodes the response body into v.
func decodeJSON(t *testing.T, resp *http.Response, v any) {
	t.Helper()
	defer resp.Body.Close()
	if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
		t.Fatalf("decode response: %v", err)
	}
}

// ── GET /health ───────────────────────────────────────────────────────────────

func TestHealth_Returns200(t *testing.T) {
	srv := newServer(&stubLLM{reply: "x"}, nil)
	resp := do(t, srv, http.MethodGet, "/health", nil)
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	var body map[string]string
	decodeJSON(t, resp, &body)
	if body["status"] != "ok" {
		t.Errorf("status field = %q, want ok", body["status"])
	}
}

func TestHealth_ContentTypeJSON(t *testing.T) {
	srv := newServer(&stubLLM{reply: "x"}, nil)
	resp := do(t, srv, http.MethodGet, "/health", nil)
	defer resp.Body.Close()
	ct := resp.Header.Get("Content-Type")
	if !strings.Contains(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
}

// ── GET /agents ───────────────────────────────────────────────────────────────

func TestListAgents_EmptyList(t *testing.T) {
	srv := newServer(&stubLLM{reply: "x"}, nil)
	resp := do(t, srv, http.MethodGet, "/agents", nil)
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	var agents []domain.Agent
	decodeJSON(t, resp, &agents)
	if len(agents) != 0 {
		t.Errorf("len = %d, want 0", len(agents))
	}
}

func TestListAgents_ReturnsSeedData(t *testing.T) {
	seed := []*domain.Agent{
		{ID: "coder", Name: "Coder", Status: domain.AgentStatusTrusted},
	}
	srv := newServer(&stubLLM{reply: "x"}, seed)
	resp := do(t, srv, http.MethodGet, "/agents", nil)
	var agents []domain.Agent
	decodeJSON(t, resp, &agents)
	if len(agents) != 1 {
		t.Fatalf("len = %d, want 1", len(agents))
	}
	if agents[0].ID != "coder" {
		t.Errorf("ID = %q, want coder", agents[0].ID)
	}
}

// ── GET /agents/{id} ──────────────────────────────────────────────────────────

func TestGetAgent_Found(t *testing.T) {
	seed := []*domain.Agent{{ID: "coder", Name: "Coder"}}
	srv := newServer(&stubLLM{reply: "x"}, seed)
	resp := do(t, srv, http.MethodGet, "/agents/coder", nil)
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	var agent domain.Agent
	decodeJSON(t, resp, &agent)
	if agent.Name != "Coder" {
		t.Errorf("Name = %q, want Coder", agent.Name)
	}
}

func TestGetAgent_NotFound_Returns404(t *testing.T) {
	srv := newServer(&stubLLM{reply: "x"}, nil)
	resp := do(t, srv, http.MethodGet, "/agents/ghost", nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", resp.StatusCode)
	}
}

// ── POST /agents/sync ─────────────────────────────────────────────────────────

func TestSyncAgents_Returns200(t *testing.T) {
	srv := newServer(&stubLLM{reply: "x"}, nil)
	resp := do(t, srv, http.MethodPost, "/agents/sync", nil)
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	var body map[string]any
	decodeJSON(t, resp, &body)
	if _, ok := body["synced"]; !ok {
		t.Error("response should contain 'synced' field")
	}
}

// ── POST /tasks ───────────────────────────────────────────────────────────────

func TestCreateTask_HappyPath(t *testing.T) {
	srv := newServer(&stubLLM{reply: "PONG"}, nil)
	resp := do(t, srv, http.MethodPost, "/tasks", map[string]string{"prompt": "PING"})
	if resp.StatusCode != http.StatusCreated {
		t.Errorf("status = %d, want 201", resp.StatusCode)
	}
	var task domain.Task
	decodeJSON(t, resp, &task)
	if task.ID == "" {
		t.Error("ID must not be empty")
	}
	if task.Status != domain.TaskStatusCompleted {
		t.Errorf("status = %q, want completed", task.Status)
	}
	if task.Output != "PONG" {
		t.Errorf("output = %q, want PONG", task.Output)
	}
}

func TestCreateTask_EmptyPrompt_Returns400(t *testing.T) {
	srv := newServer(&stubLLM{reply: "x"}, nil)
	resp := do(t, srv, http.MethodPost, "/tasks", map[string]string{"prompt": ""})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
}

func TestCreateTask_InvalidJSON_Returns400(t *testing.T) {
	srv := newServer(&stubLLM{reply: "x"}, nil)
	ts := httptest.NewServer(srv)
	defer ts.Close()

	resp, err := http.Post(ts.URL+"/tasks", "application/json", strings.NewReader("{bad json"))
	if err != nil {
		t.Fatalf("request error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
}

func TestCreateTask_LLMError_Returns201WithFailedStatus(t *testing.T) {
	// The service marks the task failed but Create() itself doesn't return an
	// error on LLM failure — so the API still returns 201 with status=failed.
	srv := newServer(&stubLLM{err: fmt.Errorf("upstream error")}, nil)
	resp := do(t, srv, http.MethodPost, "/tasks", map[string]string{"prompt": "hi"})
	if resp.StatusCode != http.StatusCreated {
		t.Errorf("status = %d, want 201", resp.StatusCode)
	}
	var task domain.Task
	decodeJSON(t, resp, &task)
	if task.Status != domain.TaskStatusFailed {
		t.Errorf("status = %q, want failed", task.Status)
	}
	if task.Error == "" {
		t.Error("Error field should be set on failure")
	}
}

// ── GET /tasks ────────────────────────────────────────────────────────────────

func TestListTasks_Empty(t *testing.T) {
	srv := newServer(&stubLLM{reply: "x"}, nil)
	resp := do(t, srv, http.MethodGet, "/tasks", nil)
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	var tasks []domain.Task
	decodeJSON(t, resp, &tasks)
	if len(tasks) != 0 {
		t.Errorf("len = %d, want 0", len(tasks))
	}
}

func TestListTasks_ReturnsPreviouslyCreated(t *testing.T) {
	srv := newServer(&stubLLM{reply: "ok"}, nil)

	// Use a single httptest.Server so all requests share the same in-memory state.
	ts := httptest.NewServer(srv)
	defer ts.Close()

	post := func(prompt string) {
		body, _ := json.Marshal(map[string]string{"prompt": prompt})
		resp, err := http.Post(ts.URL+"/tasks", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("post error: %v", err)
		}
		resp.Body.Close()
	}
	post("task-one")
	post("task-two")

	resp, err := http.Get(ts.URL + "/tasks")
	if err != nil {
		t.Fatalf("get error: %v", err)
	}
	var tasks []domain.Task
	if err := json.NewDecoder(resp.Body).Decode(&tasks); err != nil {
		t.Fatalf("decode: %v", err)
	}
	resp.Body.Close()

	if len(tasks) != 2 {
		t.Errorf("len = %d, want 2", len(tasks))
	}
}

// ── GET /tasks/{id} ───────────────────────────────────────────────────────────

func TestGetTask_Found(t *testing.T) {
	srv := newServer(&stubLLM{reply: "result"}, nil)
	ts := httptest.NewServer(srv)
	defer ts.Close()

	// Create a task.
	body, _ := json.Marshal(map[string]string{"prompt": "hello"})
	createResp, _ := http.Post(ts.URL+"/tasks", "application/json", bytes.NewReader(body))
	var created domain.Task
	json.NewDecoder(createResp.Body).Decode(&created)
	createResp.Body.Close()

	// Fetch it by ID.
	getResp, err := http.Get(ts.URL + "/tasks/" + created.ID)
	if err != nil {
		t.Fatalf("get error: %v", err)
	}
	if getResp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", getResp.StatusCode)
	}
	var task domain.Task
	json.NewDecoder(getResp.Body).Decode(&task)
	getResp.Body.Close()

	if task.ID != created.ID {
		t.Errorf("ID = %q, want %q", task.ID, created.ID)
	}
}

func TestGetTask_NotFound_Returns404(t *testing.T) {
	srv := newServer(&stubLLM{reply: "x"}, nil)
	resp := do(t, srv, http.MethodGet, "/tasks/nonexistent", nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", resp.StatusCode)
	}
}
