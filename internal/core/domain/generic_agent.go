package domain

import (
	"fmt"
	"strings"
)

// AgentType identifies the runtime implementation kind.
// Today only "generic" exists; behavior is data-driven by config files.
type AgentType string

const (
	AgentTypeGeneric AgentType = "generic"
)

// AgentConfig is the normalized config loaded from config.toml.
type AgentConfig struct {
	ID           string
	Type         AgentType
	Name         string
	Description  string
	Capabilities []string
	PromptFile   string
	Status       AgentStatus
}

// GenericAgent is the universal runtime descriptor loaded from data files.
// It combines config.toml (metadata + capabilities) with AGENT.md (system prompt).
type GenericAgent struct {
	config       AgentConfig
	systemPrompt string
}

// NewGenericAgent validates and creates a GenericAgent from config + prompt text.
func NewGenericAgent(cfg AgentConfig, systemPrompt string) (*GenericAgent, error) {
	cfg.ID = strings.TrimSpace(cfg.ID)
	if cfg.ID == "" {
		return nil, fmt.Errorf("generic agent: id is required")
	}
	if cfg.Type == "" {
		cfg.Type = AgentTypeGeneric
	}
	if cfg.Type != AgentTypeGeneric {
		return nil, fmt.Errorf("generic agent: unsupported type %q", cfg.Type)
	}
	if cfg.Name == "" {
		cfg.Name = cfg.ID
	}
	if cfg.Status == "" {
		cfg.Status = AgentStatusTrusted
	}

	return &GenericAgent{config: cfg, systemPrompt: strings.TrimSpace(systemPrompt)}, nil
}

func (a *GenericAgent) ID() string {
	return a.config.ID
}

func (a *GenericAgent) Type() AgentType {
	return a.config.Type
}

func (a *GenericAgent) Config() AgentConfig {
	cp := a.config
	cp.Capabilities = append([]string(nil), cp.Capabilities...)
	return cp
}

func (a *GenericAgent) SystemPrompt() string {
	return a.systemPrompt
}

// AsAgent returns the API/storage metadata view derived from this runtime agent.
func (a *GenericAgent) AsAgent() *Agent {
	return &Agent{
		ID:           a.config.ID,
		Type:         string(a.config.Type),
		Name:         a.config.Name,
		Description:  a.config.Description,
		Capabilities: append([]string(nil), a.config.Capabilities...),
		Status:       a.config.Status,
	}
}

// BuildMessages constructs a single-turn prompt using system prompt + user input.
func (a *GenericAgent) BuildMessages(userMessage string) []Message {
	messages := make([]Message, 0, 2)
	if a.systemPrompt != "" {
		messages = append(messages, Message{Role: RoleSystem, Content: a.systemPrompt})
	}
	messages = append(messages, Message{Role: RoleUser, Content: userMessage})
	return messages
}
