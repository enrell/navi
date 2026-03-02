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
)

type agentTOML struct {
	ID           string   `toml:"id"`
	Name         string   `toml:"name"`
	Description  string   `toml:"description"`
	Capabilities []string `toml:"capabilities"`
	Status       string   `toml:"status"`
}

// LoadAgentsFromRoots loads all agent definitions from one or more roots.
// Missing roots are ignored; malformed files return an error.
// If duplicate IDs are found, later roots overwrite earlier ones.
func LoadAgentsFromRoots(roots []string) ([]*domain.Agent, error) {
	byID := map[string]*domain.Agent{}

	for _, root := range roots {
		if root == "" {
			continue
		}

		entries, err := os.ReadDir(root)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
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
				return nil, fmt.Errorf("localfs agents: read %q: %w", cfgPath, err)
			}

			var cfg agentTOML
			if _, err := toml.Decode(string(cfgData), &cfg); err != nil {
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

			description := strings.TrimSpace(cfg.Description)
			if description == "" {
				if mdData, err := os.ReadFile(filepath.Join(agentDir, "AGENT.md")); err == nil {
					description = strings.TrimSpace(firstLine(string(mdData)))
				}
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

			byID[agentID] = &domain.Agent{
				ID:           agentID,
				Name:         name,
				Description:  description,
				Capabilities: caps,
				Status:       status,
			}
		}
	}

	ids := make([]string, 0, len(byID))
	for id := range byID {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	result := make([]*domain.Agent, 0, len(ids))
	for _, id := range ids {
		result = append(result, byID[id])
	}
	return result, nil
}

func firstLine(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	if idx := strings.IndexByte(s, '\n'); idx >= 0 {
		return s[:idx]
	}
	return s
}
