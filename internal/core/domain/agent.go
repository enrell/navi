package domain

// AgentStatus reflects how trusted an agent configuration is.
type AgentStatus string

const (
	AgentStatusTrusted   AgentStatus = "trusted"
	AgentStatusModified  AgentStatus = "modified"
	AgentStatusUntrusted AgentStatus = "untrusted"
)

// Agent represents a configured AI agent the orchestrator can delegate work to.
type Agent struct {
	ID           string      `json:"id"`
	Name         string      `json:"name"`
	Description  string      `json:"description"`
	Capabilities []string    `json:"capabilities"`
	Status       AgentStatus `json:"status"`
}
