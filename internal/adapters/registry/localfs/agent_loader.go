// Package localfs loads agent definitions from filesystem directories.
//
// Expected agent layout (per agent):
//
//	<agents_root>/<agent-id>/config.toml
//	<agents_root>/<agent-id>/AGENT.md   (optional)
package localfs

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"

	"navi/internal/core/domain"
	"navi/internal/telemetry"
)

type agentTOML struct {
	ID           string   `toml:"id"`
	Type         string   `toml:"type"`
	Prompt       string   `toml:"prompt"`
	Name         string   `toml:"name"`
	Description  string   `toml:"description"`
	Capabilities []string `toml:"capabilities"`
	Status       string   `toml:"status"`
}

// LoadGenericAgentsFromRoots loads GenericAgent definitions from one or more
// roots using config.toml + AGENT.md as the source of truth.
// Missing roots are ignored; malformed files return an error.
// If duplicate IDs are found, later roots overwrite earlier ones.
func LoadGenericAgentsFromRoots(roots []string) ([]*domain.GenericAgent, error) {
	telemetry.Logger().Info("localfs_load_agents_start", "roots", len(roots))
	byID := map[string]*domain.GenericAgent{}

	for _, root := range roots {
		if root == "" {
			continue
		}

		entries, err := os.ReadDir(root)
		if err != nil {
			if os.IsNotExist(err) {
				telemetry.Logger().Info("localfs_root_missing", "root", root)
				continue
			}
			telemetry.Logger().Error("localfs_read_root_failed", "root", root, "error", err.Error())
			return nil, fmt.Errorf("localfs agents: read dir %q: %w", root, err)
		}

		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}

			agentDir := filepath.Join(root, entry.Name())
			cfgPath := filepath.Join(agentDir, "config.toml")

			cfgData, err := os.ReadFile(cfgPath)
			if err != nil {
				if os.IsNotExist(err) {
					continue
				}
				telemetry.Logger().Error("localfs_read_config_failed", "config", cfgPath, "error", err.Error())
				return nil, fmt.Errorf("localfs agents: read %q: %w", cfgPath, err)
			}

			var cfg agentTOML
			if _, err := toml.Decode(string(cfgData), &cfg); err != nil {
				telemetry.Logger().Error("localfs_parse_config_failed", "config", cfgPath, "error", err.Error())
				return nil, fmt.Errorf("localfs agents: parse %q: %w", cfgPath, err)
			}

			agentID := strings.TrimSpace(cfg.ID)
			if agentID == "" {
				agentID = entry.Name()
			}

			name := strings.TrimSpace(cfg.Name)
			if name == "" {
				name = agentID
			}

			promptFile := strings.TrimSpace(cfg.Prompt)
			if promptFile == "" {
				promptFile = "AGENT.md"
			}

			promptPath := filepath.Join(agentDir, promptFile)
			promptData, err := os.ReadFile(promptPath)
			if err != nil {
				if os.IsNotExist(err) {
					// Backward compatibility: if config doesn't set prompt and AGENT.md
					// is missing, keep empty prompt; if prompt was explicitly set,
					// treat it as a hard error.
					if strings.TrimSpace(cfg.Prompt) != "" {
						telemetry.Logger().Error("localfs_read_prompt_failed", "prompt", promptPath, "error", err.Error())
						return nil, fmt.Errorf("localfs agents: read prompt %q: %w", promptPath, err)
					}
					promptData = nil
				} else {
					telemetry.Logger().Error("localfs_read_prompt_failed", "prompt", promptPath, "error", err.Error())
					return nil, fmt.Errorf("localfs agents: read prompt %q: %w", promptPath, err)
				}
			}

			description := strings.TrimSpace(cfg.Description)
			if description == "" {
				description = strings.TrimSpace(firstLine(string(promptData)))
			}

			status := domain.AgentStatusTrusted
			if strings.TrimSpace(cfg.Status) != "" {
				status = domain.AgentStatus(strings.ToLower(strings.TrimSpace(cfg.Status)))
			}

			caps := make([]string, 0, len(cfg.Capabilities))
			for _, c := range cfg.Capabilities {
				c = strings.TrimSpace(c)
				if c != "" {
					caps = append(caps, c)
				}
			}

			cfgType := strings.TrimSpace(strings.ToLower(cfg.Type))
			if cfgType == "" {
				cfgType = string(domain.AgentTypeGeneric)
			}

			ga, err := domain.NewGenericAgent(domain.AgentConfig{
				ID:           agentID,
				Type:         domain.AgentType(cfgType),
				Name:         name,
				Description:  description,
				Capabilities: caps,
				PromptFile:   promptFile,
				Status:       status,
			}, string(promptData))
			if err != nil {
				telemetry.Logger().Error("localfs_generic_agent_invalid", "agent_id", agentID, "error", err.Error())
				return nil, fmt.Errorf("localfs agents: %s: %w", agentID, err)
			}

			byID[agentID] = ga
			telemetry.Logger().Info("localfs_agent_loaded", "agent_id", agentID, "type", cfgType)
		}
	}

	ids := make([]string, 0, len(byID))
	for id := range byID {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	result := make([]*domain.GenericAgent, 0, len(ids))
	for _, id := range ids {
		result = append(result, byID[id])
	}
	telemetry.Logger().Info("localfs_load_agents_done", "count", len(result))
	return result, nil
}

// LoadAgentsFromRoots is a compatibility wrapper that returns only metadata.
func LoadAgentsFromRoots(roots []string) ([]*domain.Agent, error) {
	genericAgents, err := LoadGenericAgentsFromRoots(roots)
	if err != nil {
		return nil, err
	}
	out := make([]*domain.Agent, 0, len(genericAgents))
	for _, ga := range genericAgents {
		out = append(out, ga.AsAgent())
	}
	return out, nil
}

func firstLine(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	if idx := strings.IndexByte(s, '\n'); idx >= 0 {
		return s[:idx]
	}
	return s
}
