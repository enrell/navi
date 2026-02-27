package capabilities

import (
	"fmt"
	"strings"

	"navi/internal/core/domain"
)

// Parse converts a slice of raw capability strings into typed Capability values.
// It delegates to domain.ParseCapability and collects all errors.
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

// Satisfies reports whether agentCaps is a superset of required.
// An empty required slice is always satisfied.
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
		// Wildcard or exact resource match
		if req.Resource == "" || req.Resource == "*" {
			return true
		}
		if c.Resource == "*" || strings.EqualFold(c.Resource, req.Resource) {
			return true
		}
	}
	return false
}
