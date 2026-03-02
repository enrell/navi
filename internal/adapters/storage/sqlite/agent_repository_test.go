package sqlite_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	sqliteadapter "navi/internal/adapters/storage/sqlite"
	"navi/internal/core/domain"
)

func newAgentRepo(t *testing.T) *sqliteadapter.AgentRepository {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "agents.db")
	repo, err := sqliteadapter.NewAgentRepository(dbPath)
	if err != nil {
		t.Fatalf("NewAgentRepository error: %v", err)
	}
	t.Cleanup(func() {
		if err := repo.Close(); err != nil {
			t.Fatalf("Close error: %v", err)
		}
	})
	return repo
}

func TestNewAgentRepository_EmptyPathError(t *testing.T) {
	_, err := sqliteadapter.NewAgentRepository("")
	if err == nil {
		t.Fatal("expected error for empty db path")
	}
}

func TestSeedAndFindByID(t *testing.T) {
	repo := newAgentRepo(t)
	seed := []*domain.Agent{{
		ID:           "coder",
		Type:         "generic",
		Name:         "Coder",
		Description:  "Writes code",
		Capabilities: []string{"filesystem:workspace:rw", "exec:go,git"},
		Status:       domain.AgentStatusTrusted,
	}}

	if err := repo.Seed(context.Background(), seed); err != nil {
		t.Fatalf("Seed error: %v", err)
	}

	got, err := repo.FindByID(context.Background(), "coder")
	if err != nil {
		t.Fatalf("FindByID error: %v", err)
	}
	if got.Name != "Coder" {
		t.Errorf("Name = %q, want Coder", got.Name)
	}
	if got.Type != "generic" {
		t.Errorf("Type = %q, want generic", got.Type)
	}
	if len(got.Capabilities) != 2 {
		t.Fatalf("capabilities len = %d, want 2", len(got.Capabilities))
	}
	if got.Capabilities[0] != "filesystem:workspace:rw" {
		t.Errorf("capability[0] = %q, want filesystem:workspace:rw", got.Capabilities[0])
	}
}

func TestSeed_Upsert(t *testing.T) {
	repo := newAgentRepo(t)
	ctx := context.Background()

	if err := repo.Seed(ctx, []*domain.Agent{{ID: "coder", Name: "Coder", Status: domain.AgentStatusTrusted}}); err != nil {
		t.Fatalf("seed 1 error: %v", err)
	}
	if err := repo.Seed(ctx, []*domain.Agent{{ID: "coder", Name: "Coder v2", Status: domain.AgentStatusModified}}); err != nil {
		t.Fatalf("seed 2 error: %v", err)
	}

	got, err := repo.FindByID(ctx, "coder")
	if err != nil {
		t.Fatalf("FindByID error: %v", err)
	}
	if got.Name != "Coder v2" {
		t.Errorf("Name = %q, want Coder v2", got.Name)
	}
	if got.Status != domain.AgentStatusModified {
		t.Errorf("Status = %q, want modified", got.Status)
	}
}

func TestAgentFindByID_NotFound(t *testing.T) {
	repo := newAgentRepo(t)
	_, err := repo.FindByID(context.Background(), "missing")
	if err == nil {
		t.Fatal("expected not found error")
	}
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("error = %v, want ErrNotFound in chain", err)
	}
}

func TestFindAll_OrderedByID(t *testing.T) {
	repo := newAgentRepo(t)
	ctx := context.Background()
	seed := []*domain.Agent{
		{ID: "researcher", Name: "Researcher", Status: domain.AgentStatusTrusted},
		{ID: "coder", Name: "Coder", Status: domain.AgentStatusTrusted},
	}
	if err := repo.Seed(ctx, seed); err != nil {
		t.Fatalf("Seed error: %v", err)
	}

	all, err := repo.FindAll(ctx)
	if err != nil {
		t.Fatalf("FindAll error: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("len = %d, want 2", len(all))
	}
	if all[0].ID != "coder" || all[1].ID != "researcher" {
		t.Errorf("order = [%s %s], want [coder researcher]", all[0].ID, all[1].ID)
	}
}

func TestAgentFindAll_Empty(t *testing.T) {
	repo := newAgentRepo(t)
	all, err := repo.FindAll(context.Background())
	if err != nil {
		t.Fatalf("FindAll error: %v", err)
	}
	if len(all) != 0 {
		t.Errorf("len = %d, want 0", len(all))
	}
}
