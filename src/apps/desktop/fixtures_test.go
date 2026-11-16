package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestWriteFrontendFixtures writes the views the frontend tests render
// (frontend/src/testing/fixtures) from the real Go services. It runs only
// when OTTER_FIXTURES names the folder to write to:
//
//	OTTER_FIXTURES=$PWD/frontend/src/testing/fixtures go test -run TestWriteFrontendFixtures .
func TestWriteFrontendFixtures(t *testing.T) {
	dir := os.Getenv("OTTER_FIXTURES")
	if dir == "" {
		t.Skip("set OTTER_FIXTURES to write the frontend fixtures")
	}
	fixtures := map[string]any{
		"app-info": AppInfo{Version: "1.0.0"},
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
