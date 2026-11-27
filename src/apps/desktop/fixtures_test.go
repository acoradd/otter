package main

import (
	"encoding/json"
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
	db := openDB(t)
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

	fixtures := map[string]any{
		"app-info":         AppInfo{Version: "1.0.0"},
		"settings":         must(settings.GetSettings(ctx)),
		"workspaces":       must(workspaces.ListWorkspaces(ctx)),
		"workspace-view":   must(workspaces.GetWorkspaceView(ctx, payments)),
		"environment-view": must(workspaces.GetEnvironmentView(ctx, dev)),
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
