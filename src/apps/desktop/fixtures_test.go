package main

import (
	"database/sql"
	"encoding/json"

	"github.com/acoradd/otter/src/internal/store"
	"os"
	"path/filepath"
	"testing"
)

// TestWriteFrontendFixtures writes the views the frontend tests render
// (frontend/src/testing/fixtures) from the real Go services, on a demo
// workspace. It runs only when OTTER_FIXTURES names the folder to write to:
//
//	OTTER_FIXTURES=$PWD/frontend/src/testing/fixtures go test -run TestWriteFrontendFixtures .
func TestWriteFrontendFixtures(t *testing.T) {
	dir := os.Getenv("OTTER_FIXTURES")
	if dir == "" {
		t.Skip("set OTTER_FIXTURES to write the frontend fixtures")
	}
	path := filepath.Join(t.TempDir(), store.FileName)
	db, err := store.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	rec := &emitted{}
	workspaces := NewWorkspaceService(db, rec.emit)
	settings := NewSettingsService(db, rec.emit)

	payments := must(workspaces.CreateWorkspace(ctx, "Payments API"))
	must(workspaces.CreateWorkspace(ctx, "Identity"))
	dev := must(workspaces.CreateEnvironment(ctx, payments, "dev"))
	must(workspaces.CreateEnvironment(ctx, payments, "staging"))
	must(workspaces.CreateEnvironment(ctx, payments, "production"))
	if err := workspaces.SaveEnvironmentVars(ctx, dev, []EnvVarRow{
		{Key: "baseUrl", Value: "https://dev.payments.test", Enabled: true},
		{Key: "apiKey", Value: "dev-4f9a2c", Secret: true, Enabled: true},
		{Key: "tenant", Value: "acme", Enabled: false},
	}); err != nil {
		t.Fatal(err)
	}
	if err := workspaces.SetActiveEnvironment(ctx, payments, dev); err != nil {
		t.Fatal(err)
	}
	if err := workspaces.OpenWorkspace(ctx, payments); err != nil {
		t.Fatal(err)
	}

	tree := NewTreeService(db, rec.emit)
	customers := must(tree.CreateFolder(ctx, payments, 0, "customers"))
	paymentsFolder := must(tree.CreateFolder(ctx, payments, 0, "payments"))
	must(tree.CreateFolder(ctx, payments, 0, "webhooks"))
	cards := must(tree.CreateFolder(ctx, payments, customers, "cards"))
	must(tree.CreateRequest(ctx, customers, "List customers"))
	createCustomer := must(tree.CreateRequest(ctx, customers, "Create customer"))
	deleteCard := must(tree.CreateRequest(ctx, cards, "Delete card"))
	refund := must(tree.CreateRequest(ctx, paymentsFolder, "Refund payment"))
	// Until the request editor (step 3) has a command for it.
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	for id, method := range map[int64]string{createCustomer: "POST", deleteCard: "DELETE", refund: "POST"} {
		if _, err := raw.Exec(`UPDATE request SET method = ? WHERE id = ?`, method, id); err != nil {
			t.Fatal(err)
		}
	}
	raw.Close()

	fixtures := map[string]any{
		"app-info":         AppInfo{Version: "1.0.0"},
		"settings":         must(settings.GetSettings(ctx)),
		"workspaces":       must(workspaces.ListWorkspaces(ctx)),
		"workspace-view":   must(workspaces.GetWorkspaceView(ctx, payments)),
		"environment-view": must(workspaces.GetEnvironmentView(ctx, dev)),
		"tree-root":        must(tree.GetChildren(ctx, payments, 0)),
		"tree-customers":   must(tree.GetChildren(ctx, payments, customers)),
		"tree-cards":       must(tree.GetChildren(ctx, payments, cards)),
		"folder-choices":   must(tree.GetFolderChoices(ctx, payments)),
	}
	for name, view := range fixtures {
		b, err := json.MarshalIndent(view, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name+".json"), append(b, '\n'), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}
