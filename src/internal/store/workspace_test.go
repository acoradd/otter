package store

import (
	"testing"

	"github.com/acoradd/otter/src/internal/apperr"
)

func TestWorkspaceLifecycle(t *testing.T) {
	db := open(t)
	b, err := db.CreateWorkspace(ctx, "  billing ")
	if err != nil {
		t.Fatal(err)
	}
	a, _ := db.CreateWorkspace(ctx, "Auth")

	list, err := db.Workspaces(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 || list[0].ID != a || list[1].ID != b || list[1].Name != "billing" {
		t.Fatalf("workspaces = %+v: want Auth, billing (trimmed, by name ignoring case)", list)
	}

	if err := db.RenameWorkspace(ctx, b, "Billing"); err != nil {
		t.Fatal(err)
	}
	if w, err := db.Workspace(ctx, b); err != nil || w.Name != "Billing" || w.ActiveEnvironmentID != 0 {
		t.Fatalf("Workspace = %+v, %v", w, err)
	}

	if err := db.DeleteWorkspace(ctx, b); err != nil {
		t.Fatal(err)
	}
	_, err = db.Workspace(ctx, b)
	wantCode(t, err, apperr.NotFound)
}

func TestWorkspaceErrors(t *testing.T) {
	db := open(t)
	id, _ := db.CreateWorkspace(ctx, "API")
	tests := []struct {
		name string
		err  error
		want apperr.Code
	}{
		{"create without a name", func() error { _, err := db.CreateWorkspace(ctx, "   "); return err }(), CodeNameRequired},
		{"rename to nothing", db.RenameWorkspace(ctx, id, ""), CodeNameRequired},
		{"rename a missing one", db.RenameWorkspace(ctx, 999, "x"), apperr.NotFound},
		{"delete a missing one", db.DeleteWorkspace(ctx, 999), apperr.NotFound},
		{"activate in a missing one", db.SetActiveEnvironment(ctx, 999, 0), apperr.NotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) { wantCode(t, tt.err, tt.want) })
	}
}

func TestSetActiveEnvironment(t *testing.T) {
	db := open(t)
	ws, _ := db.CreateWorkspace(ctx, "API")
	other, _ := db.CreateWorkspace(ctx, "Other")
	dev, _ := db.CreateEnvironment(ctx, ws, "dev")
	foreign, _ := db.CreateEnvironment(ctx, other, "prod")

	active := func() int64 {
		t.Helper()
		w, err := db.Workspace(ctx, ws)
		if err != nil {
			t.Fatal(err)
		}
		return w.ActiveEnvironmentID
	}

	if err := db.SetActiveEnvironment(ctx, ws, dev); err != nil || active() != dev {
		t.Fatalf("activate dev: %v, active %d", err, active())
	}
	wantCode(t, db.SetActiveEnvironment(ctx, ws, foreign), apperr.InvalidInput)
	wantCode(t, db.SetActiveEnvironment(ctx, ws, 999), apperr.NotFound)
	if active() != dev {
		t.Fatalf("a refused change kept dev active? active %d", active())
	}

	// Deleting the active environment leaves none active.
	if err := db.DeleteEnvironment(ctx, dev); err != nil {
		t.Fatal(err)
	}
	if active() != 0 {
		t.Fatalf("active = %d after deleting it, want 0", active())
	}

	if err := db.SetActiveEnvironment(ctx, other, foreign); err != nil {
		t.Fatal(err)
	}
	if err := db.SetActiveEnvironment(ctx, other, 0); err != nil {
		t.Fatal(err)
	}
	if w, _ := db.Workspace(ctx, other); w.ActiveEnvironmentID != 0 {
		t.Fatalf("active = %d after clearing, want 0", w.ActiveEnvironmentID)
	}
}
