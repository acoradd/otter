package store

import (
	"testing"
	"time"
)

func TestSeedDemo(t *testing.T) {
	db := open(t)
	ws, err := db.SeedDemo(ctx, "Demo", DemoSize{Requests: 100, Depth: 3, Branching: 2})
	if err != nil {
		t.Fatal(err)
	}
	var folders, requests, maxDepth int
	db.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM folder WHERE workspace_id = ?`, ws).Scan(&folders)
	db.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM request r JOIN folder f ON f.id = r.folder_id WHERE f.workspace_id = ?`, ws).Scan(&requests)
	db.db.QueryRowContext(ctx, `WITH RECURSIVE d(id, depth) AS (
	    SELECT id, 1 FROM folder WHERE workspace_id = ? AND parent_id IS NULL
	    UNION ALL SELECT f.id, d.depth + 1 FROM folder f JOIN d ON f.parent_id = d.id
	  ) SELECT MAX(depth) FROM d`, ws).Scan(&maxDepth)
	if folders != 2+4+8 || requests != 100 || maxDepth != 3 {
		t.Fatalf("folders %d, requests %d, depth %d: want 14, 100, 3", folders, requests, maxDepth)
	}
	w, _ := db.Workspace(ctx, ws)
	vars, _ := db.EnvVars(ctx, w.ActiveEnvironmentID)
	if len(vars) != 1 || vars[0].Key != "baseUrl" {
		t.Fatalf("demo environment vars = %+v", vars)
	}
}

// TestLargeTreeStaysFast seeds the large demo and checks that loading the
// children of a folder, which is what expanding it does, stays far below
// what a user notices.
func TestLargeTreeStaysFast(t *testing.T) {
	if testing.Short() {
		t.Skip("seeds 10 000 requests")
	}
	db := open(t)
	start := time.Now()
	ws, err := db.SeedDemo(ctx, "Large", LargeDemo)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("seeded %d requests in %s", LargeDemo.Requests, time.Since(start))

	start = time.Now()
	const rounds = 50
	folder := int64(0)
	for i := 0; i < rounds; i++ {
		nodes, err := db.Children(ctx, ws, folder)
		if err != nil {
			t.Fatal(err)
		}
		if len(nodes) == 0 {
			t.Fatal("no children")
		}
		if nodes[0].Kind == KindFolder {
			folder = nodes[0].ID // walk down, then start again at the root
		} else {
			folder = 0
		}
	}
	per := time.Since(start) / rounds
	t.Logf("Children: %s per folder", per)
	if per > 20*time.Millisecond {
		t.Errorf("Children takes %s per folder, want under 20ms", per)
	}
}
