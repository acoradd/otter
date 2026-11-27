package store

import (
	"context"
	"database/sql"
	"fmt"
)

// DemoSize shapes a demo workspace: Branching folders per folder over Depth
// levels, with Requests spread over every folder.
type DemoSize struct {
	Requests  int
	Depth     int
	Branching int
}

// LargeDemo is the size the tree and search must stay fast with
// (docs/specs/scope-v1.md, step 2): 10 000 requests, 6 levels deep.
var LargeDemo = DemoSize{Requests: 10000, Depth: 6, Branching: 3}

var demoMethods = []string{"GET", "GET", "POST", "PUT", "PATCH", "DELETE", "GET"}

// SeedDemo creates a demo workspace of the given size, with a "demo"
// environment defining baseUrl, in one transaction, and returns its ID.
// Names are placeholders for development, not user-facing text.
func (d *DB) SeedDemo(ctx context.Context, name string, size DemoSize) (int64, error) {
	var ws int64
	err := d.tx(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `INSERT INTO workspace (name) VALUES (?)`, name)
		if err != nil {
			return err
		}
		if ws, err = res.LastInsertId(); err != nil {
			return err
		}
		res, err = tx.ExecContext(ctx, `INSERT INTO environment (workspace_id, name) VALUES (?, 'demo')`, ws)
		if err != nil {
			return err
		}
		env, _ := res.LastInsertId()
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO env_var (environment_id, key, value) VALUES (?, 'baseUrl', 'https://api.example.test')`, env); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE workspace SET active_environment_id = ? WHERE id = ?`, env, ws); err != nil {
			return err
		}

		folder, err := tx.PrepareContext(ctx, `INSERT INTO folder (workspace_id, parent_id, name, seq) VALUES (?, ?, ?, ?)`)
		if err != nil {
			return err
		}
		defer folder.Close()
		var folders []int64
		level := []struct {
			id   int64
			path string
		}{{0, ""}}
		for depth := 1; depth <= size.Depth; depth++ {
			var next []struct {
				id   int64
				path string
			}
			for _, parent := range level {
				for i := 1; i <= size.Branching; i++ {
					path := fmt.Sprintf("%s%d", parent.path, i)
					res, err := folder.ExecContext(ctx, ws, nullID(parent.id), "Folder "+path, i-1)
					if err != nil {
						return err
					}
					id, _ := res.LastInsertId()
					folders = append(folders, id)
					next = append(next, struct {
						id   int64
						path string
					}{id, path + "."})
				}
			}
			level = next
		}
		if len(folders) == 0 {
			return nil
		}

		request, err := tx.PrepareContext(ctx,
			`INSERT INTO request (folder_id, seq, name, method, url) VALUES (?, ?, ?, ?, ?)`)
		if err != nil {
			return err
		}
		defer request.Close()
		for i := 0; i < size.Requests; i++ {
			method := demoMethods[i%len(demoMethods)]
			url := fmt.Sprintf("{{baseUrl}}/v1/resources-%d/items/%d", i%97, i)
			if _, err := request.ExecContext(ctx, folders[i%len(folders)], i/len(folders), fmt.Sprintf("%s item %d", method, i), method, url); err != nil {
				return err
			}
		}
		return nil
	})
	return ws, err
}
