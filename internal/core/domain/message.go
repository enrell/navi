// Package domain contains the core business types for Navi.
// It has zero external dependencies — no frameworks, no adapters, no I/O.
package domain

// Role identifies the participant in a conversation turn.
type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
)

// Message is a single turn in a conversation.
type Message struct {
	Role    Role
	Content string
}
