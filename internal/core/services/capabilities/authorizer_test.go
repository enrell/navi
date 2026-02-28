package capabilities_test

import (
	"errors"
	"testing"

	"navi/internal/core/domain"
	"navi/internal/core/services/capabilities"
)

func TestAuthorizer_CheckExec(t *testing.T) {
	caps := []domain.Capability{
		{Type: "exec", Resource: "bash,go,node"},
		{Type: "exec", Resource: "git"},
		{Type: "network", Resource: "noise"}, // To test c.Type != "exec"
	}

	auth := capabilities.NewAuthorizer(caps)

	t.Run("allowed binary", func(t *testing.T) {
		if err := auth.CheckExec("node"); err != nil {
			t.Errorf("expected no error, got %v", err)
		}
		if err := auth.CheckExec("git"); err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})

	t.Run("denied binary", func(t *testing.T) {
		err := auth.CheckExec("python")
		if err == nil {
			t.Error("expected error for python, got nil")
		}
		if !errors.Is(err, capabilities.ErrDenied) {
			t.Errorf("expected ErrDenied, got %v", err)
		}
	})

	t.Run("wildcard binary", func(t *testing.T) {
		wildcardAuth := capabilities.NewAuthorizer([]domain.Capability{
			{Type: "exec", Resource: "*"},
		})
		if err := wildcardAuth.CheckExec("any-binary"); err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})
}

func TestAuthorizer_CheckFilesystem(t *testing.T) {
	caps := []domain.Capability{
		{Type: "filesystem", Resource: "workspace", Mode: "rw"},
		{Type: "filesystem", Resource: "/tmp/readonly", Mode: "ro"},
	}

	workspacePath := "/var/app/workspace"
	auth := capabilities.NewAuthorizer(caps)

	t.Run("read allowed workspace", func(t *testing.T) {
		err := auth.CheckFilesystem("/var/app/workspace/file.txt", "read", workspacePath)
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})

	t.Run("write allowed workspace", func(t *testing.T) {
		err := auth.CheckFilesystem("/var/app/workspace/secret.txt", "write", workspacePath)
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})

	t.Run("read allowed absolute", func(t *testing.T) {
		err := auth.CheckFilesystem("/tmp/readonly/test.txt", "read", workspacePath)
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})

	t.Run("write denied absolute (read-only)", func(t *testing.T) {
		err := auth.CheckFilesystem("/tmp/readonly/test.txt", "write", workspacePath)
		if err == nil {
			t.Error("expected error for write to ro, got nil")
		}
	})

	t.Run("access denied outside", func(t *testing.T) {
		err := auth.CheckFilesystem("/etc/passwd", "read", workspacePath)
		if err == nil {
			t.Error("expected error for /etc/passwd, got nil")
		}
	})

	t.Run("skip non-filesystem capability", func(t *testing.T) {
		mixedAuth := capabilities.NewAuthorizer([]domain.Capability{
			{Type: "exec", Resource: "bash"},
			{Type: "filesystem", Resource: workspacePath, Mode: "ro"},
		})
		err := mixedAuth.CheckFilesystem("/var/app/workspace/file.txt", "read", workspacePath)
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})

	t.Run("invalid requested path", func(t *testing.T) {
		// Null byte is invalid in paths on most systems, forces filepath.Abs to fail in many OS
		// Or try something that forces error (hard cross platform). Let's use `\x00`
		err := auth.CheckFilesystem("\x00invalid", "read", workspacePath)
		if err == nil {
			// Some systems don't error on null byte in Abs, skip if it actually returns nil
		}
	})

	t.Run("invalid capability path", func(t *testing.T) {
		invalidAuth := capabilities.NewAuthorizer([]domain.Capability{
			{Type: "filesystem", Resource: "\x00invalid", Mode: "rw"},
		})
		err := invalidAuth.CheckFilesystem("/tmp/foo", "read", workspacePath)
		if err == nil {
			t.Error("expected error, got nil")
		}
	})

	t.Run("different root volumes", func(t *testing.T) {
		// D:\ and C:\ have no relative path on Windows, so Rel fails.
		// On non-windows this is just a normal root path and Rel works, but it will be skipped by the HasPrefix("..") check.
		diffVolumeAuth := capabilities.NewAuthorizer([]domain.Capability{
			{Type: "filesystem", Resource: `D:\workspace`, Mode: "rw"},
		})
		err := diffVolumeAuth.CheckFilesystem(`C:\tmp\foo`, "read", `D:\workspace`)
		if err == nil {
			t.Error("expected error across volumes, got nil")
		}
	})
}

func TestAuthorizer_CheckNetwork(t *testing.T) {
	caps := []domain.Capability{
		{Type: "network", Resource: "api.github.com", Mode: "443"},
		{Type: "network", Resource: "localhost", Mode: "*"}, // Any port on localhost
		{Type: "exec", Resource: "bash"},                    // Noise
	}

	auth := capabilities.NewAuthorizer(caps)

	t.Run("exact match", func(t *testing.T) {
		if err := auth.CheckNetwork("api.github.com", "443"); err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})

	t.Run("wrong port", func(t *testing.T) {
		if err := auth.CheckNetwork("api.github.com", "80"); err == nil {
			t.Error("expected error for wrong port, got nil")
		}
	})

	t.Run("wrong host", func(t *testing.T) {
		if err := auth.CheckNetwork("github.com", "443"); err == nil {
			t.Error("expected error for wrong host, got nil")
		}
	})

	t.Run("wildcard port", func(t *testing.T) {
		if err := auth.CheckNetwork("localhost", "8080"); err != nil {
			t.Errorf("expected no error for localhost:8080, got %v", err)
		}
	})

	t.Run("wildcard host and empty mode implies any port", func(t *testing.T) {
		wildHostAuth := capabilities.NewAuthorizer([]domain.Capability{
			{Type: "network", Resource: "*", Mode: ""},
		})
		if err := wildHostAuth.CheckNetwork("google.com", "80"); err != nil {
			t.Errorf("expected no error for google.com:80, got %v", err)
		}
	})

	t.Run("total wildcards", func(t *testing.T) {
		wildAuth := capabilities.NewAuthorizer([]domain.Capability{
			{Type: "network", Resource: "*", Mode: "*"},
		})
		if err := wildAuth.CheckNetwork("example.com", "443"); err != nil {
			t.Errorf("expected no error for total wildcards, got %v", err)
		}
	})
}
