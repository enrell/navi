package capabilities

import (
	"fmt"
	"strings"

	"navi/internal/core/domain"
)

func Parse(rawCaps []string) ([]domain.Capability, error) {
	caps := make([]domain.Capability, 0, len(rawCaps))
	for _, s := range rawCaps {
		c, err := domain.ParseCapability(s)
		if err != nil {
			return nil, fmt.Errorf("invalid capability %q: %w", s, err)
		}
		caps = append(caps, c)
	}
	return caps, nil
}

func Satisfies(agentCaps, required []domain.Capability) bool {
	for _, req := range required {
		if !has(agentCaps, req) {
			return false
		}
	}
	return true
}

func has(caps []domain.Capability, req domain.Capability) bool {
	for _, c := range caps {
		if c.Type != req.Type {
			continue
		}
		if req.Resource == "" || req.Resource == "*" {
			return true
		}
		if c.Resource == "*" || strings.EqualFold(c.Resource, req.Resource) {
			return true
		}
	}
	return false
}
