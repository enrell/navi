package capabilities

import (
	"errors"
	"path/filepath"
	"strings"

	"navi/internal/core/domain"
)

var ErrDenied = errors.New("action denied by capabilities")

type Authorizer struct {
	caps []domain.Capability
}

func NewAuthorizer(caps []domain.Capability) *Authorizer {
	return &Authorizer{caps: caps}
}

// CheckExec verifies if the given binary is permitted to execute
func (a *Authorizer) CheckExec(binary string) error {
	for _, c := range a.caps {
		if c.Type != "exec" {
			continue
		}
		if c.Resource == "*" {
			return nil
		}
		// Resource can be comma separated list "bash,node,go"
		allowedBinaries := strings.Split(c.Resource, ",")
		for _, b := range allowedBinaries {
			b = strings.TrimSpace(b)
			if b == binary {
				return nil
			}
		}
	}
	return ErrDenied
}

// CheckFilesystem verifies if the requested action ("read" or "write") is permitted on reqPath
func (a *Authorizer) CheckFilesystem(reqPath string, action string, workspacePath string) error {
	reqAbs, err := filepath.Abs(reqPath)
	if err != nil {
		return err // Cannot resolve path securely
	}

	for _, c := range a.caps {
		if c.Type != "filesystem" {
			continue
		}

		allowedPath := c.Resource
		if allowedPath == "workspace" {
			allowedPath = workspacePath
		}

		capAbs, err := filepath.Abs(allowedPath)
		if err != nil {
			continue
		}

		// Determine if reqAbs is inside capAbs securely
		rel, err := filepath.Rel(capAbs, reqAbs)
		if err != nil || strings.HasPrefix(rel, "..") {
			continue // Not under this capability path
		}

		// Check permissions based on action
		// Assuming modes: "rw" (read/write), "ro" (read only)
		if action == "write" && c.Mode != "rw" {
			continue
		}

		// If we reached here, this capability grants access
		return nil
	}
	return ErrDenied
}

// CheckNetwork verifies if connecting to host:port is permitted
func (a *Authorizer) CheckNetwork(host, port string) error {
	for _, c := range a.caps {
		if c.Type != "network" {
			continue
		}

		// If both host and port are wildcards
		if c.Resource == "*" && (c.Mode == "*" || c.Mode == "") {
			return nil
		}

		hostMatches := (c.Resource == "*" || strings.EqualFold(c.Resource, host))

		// For network capabilities, Mode implies the port
		modeMatches := (c.Mode == "*" || c.Mode == "" || c.Mode == port)

		if hostMatches && modeMatches {
			return nil
		}
	}
	return ErrDenied
}
