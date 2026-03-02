// Package httpserver implements the REST API adapter using go-chi.
//
// All endpoints are methods on *Server, making the handler tree fully testable
// via httptest without starting a real listener.
package httpserver

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"navi/internal/core/domain"
	agentsvc "navi/internal/core/services/agent"
	tasksvc "navi/internal/core/services/task"
)

// Server is the HTTP adapter for Navi's REST API.
// It implements http.Handler so it can be used directly with httptest.NewServer.
type Server struct {
	handler http.Handler
	tasks   *tasksvc.Service
	agents  *agentsvc.Service
	httpSrv *http.Server
}

// New wires the two services into a Server and builds the chi router.
func New(tasks *tasksvc.Service, agents *agentsvc.Service) *Server {
	s := &Server{tasks: tasks, agents: agents}
	s.handler = s.buildRouter()
	return s
}

// ServeHTTP implements http.Handler, delegating to the chi router.
// This makes *Server usable directly with httptest.NewServer(s).
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.handler.ServeHTTP(w, r)
}

// Start binds to addr (e.g. ":8080") and blocks until the server is closed.
// Returns nil on graceful shutdown via Shutdown().
func (s *Server) Start(addr string) error {
	s.httpSrv = &http.Server{
		Addr:         addr,
		Handler:      s.handler,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 120 * time.Second, // longer to accommodate LLM calls
		IdleTimeout:  120 * time.Second,
	}

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("server: listen %s: %w", addr, err)
	}

	if err := s.httpSrv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("server: serve: %w", err)
	}
	return nil
}

// buildRouter constructs the chi router with all routes registered.
func (s *Server) buildRouter() http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.Recoverer)

	r.Get("/health", s.handleHealth)

	r.Route("/agents", func(r chi.Router) {
		r.Get("/", s.handleListAgents)
		r.Post("/sync", s.handleSyncAgents)
		r.Get("/{id}", s.handleGetAgent)
	})

	r.Route("/tasks", func(r chi.Router) {
		r.Post("/", s.handleCreateTask)
		r.Get("/", s.handleListTasks)
		r.Get("/{id}", s.handleGetTask)
	})

	return r
}

// ── helpers ───────────────────────────────────────────────────────────────────

// writeJSON encodes v as JSON and writes it with the given status code.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// writeError writes a JSON error body: {"error": "..."}.
func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// httpStatus maps domain errors to HTTP status codes.
func httpStatus(err error) int {
	if errors.Is(err, domain.ErrNotFound) {
		return http.StatusNotFound
	}
	return http.StatusInternalServerError
}

// ── handlers ──────────────────────────────────────────────────────────────────

// GET /health
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// GET /agents
func (s *Server) handleListAgents(w http.ResponseWriter, r *http.Request) {
	agents, err := s.agents.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, agents)
}

// GET /agents/{id}
func (s *Server) handleGetAgent(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	agent, err := s.agents.Get(r.Context(), id)
	if err != nil {
		writeError(w, httpStatus(err), err.Error())
		return
	}
	writeJSON(w, http.StatusOK, agent)
}

// POST /agents/sync
func (s *Server) handleSyncAgents(w http.ResponseWriter, r *http.Request) {
	// TODO: implement GitHub sync once the agent registry is in place.
	writeJSON(w, http.StatusOK, map[string]any{
		"synced":  0,
		"message": "agent sync not yet implemented",
	})
}

// createTaskRequest is the JSON body for POST /tasks.
type createTaskRequest struct {
	ID      string `json:"id"`
	AgentID string `json:"agent_id"`
	Prompt  string `json:"prompt"`
}

// POST /tasks
func (s *Server) handleCreateTask(w http.ResponseWriter, r *http.Request) {
	var req createTaskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}
	if req.Prompt == "" {
		writeError(w, http.StatusBadRequest, "prompt must not be empty")
		return
	}

	task, err := s.tasks.Create(r.Context(), req.Prompt, req.AgentID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, task)
}

// GET /tasks
func (s *Server) handleListTasks(w http.ResponseWriter, r *http.Request) {
	tasks, err := s.tasks.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, tasks)
}

// GET /tasks/{id}
func (s *Server) handleGetTask(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	task, err := s.tasks.Get(r.Context(), id)
	if err != nil {
		writeError(w, httpStatus(err), err.Error())
		return
	}
	writeJSON(w, http.StatusOK, task)
}
