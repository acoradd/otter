package store

import (
	"context"

	"github.com/acoradd/otter/src/internal/apperr"
)

// ChainFolder is a folder of a request's chain with its own variables and
// headers.
type ChainFolder struct {
	ID      int64
	Name    string
	Vars    []Var
	Headers []Header
}

// chainCTE walks from a folder up to the root; depth 0 is the folder.
const chainCTE = `WITH RECURSIVE chain(id, parent_id, depth) AS (
  SELECT id, parent_id, 0 FROM folder WHERE id = ?
  UNION ALL
  SELECT f.id, f.parent_id, c.depth + 1 FROM folder f JOIN chain c ON f.id = c.parent_id
) `

// FolderChain returns the folders from the root down to folderID, with
// their enabled and disabled variables and headers: one query per kind
// (docs/specs/scope-v1.md, section 4).
func (d *DB) FolderChain(ctx context.Context, folderID int64) ([]ChainFolder, error) {
	rows, err := d.db.QueryContext(ctx, chainCTE+
		`SELECT f.id, f.name FROM chain c JOIN folder f ON f.id = c.id ORDER BY c.depth DESC`, folderID)
	if err != nil {
		return nil, err
	}
	var chain []ChainFolder
	index := map[int64]int{}
	for rows.Next() {
		var f ChainFolder
		if err := rows.Scan(&f.ID, &f.Name); err != nil {
			rows.Close()
			return nil, err
		}
		f.Vars, f.Headers = []Var{}, []Header{}
		index[f.ID] = len(chain)
		chain = append(chain, f)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	rows, err = d.db.QueryContext(ctx, chainCTE+
		`SELECT c.id, x.key, x.value, x.enabled FROM chain c JOIN folder_var x ON x.folder_id = c.id
		 ORDER BY c.depth DESC, x.seq, x.id`, folderID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var id int64
		var v Var
		if err := rows.Scan(&id, &v.Key, &v.Value, &v.Enabled); err != nil {
			rows.Close()
			return nil, err
		}
		f := &chain[index[id]]
		f.Vars = append(f.Vars, v)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	rows, err = d.db.QueryContext(ctx, chainCTE+
		`SELECT c.id, x.key, x.value, x.mode, x.enabled FROM chain c JOIN folder_header x ON x.folder_id = c.id
		 ORDER BY c.depth DESC, x.seq, x.id`, folderID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var h Header
		var mode string
		if err := rows.Scan(&id, &h.Key, &h.Value, &mode, &h.Enabled); err != nil {
			return nil, err
		}
		h.Ignore = mode == "ignore"
		f := &chain[index[id]]
		f.Headers = append(f.Headers, h)
	}
	return chain, rows.Err()
}

// RuntimeVars returns the runtime variables of a workspace for an
// environment (0: none active), by name.
func (d *DB) RuntimeVars(ctx context.Context, workspaceID, envID int64) ([]Var, error) {
	rows, err := d.db.QueryContext(ctx,
		`SELECT key, value FROM runtime_var WHERE workspace_id = ? AND environment_id = ? ORDER BY key`, workspaceID, envID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []Var{}
	for rows.Next() {
		v := Var{Enabled: true}
		if err := rows.Scan(&v.Key, &v.Value); err != nil {
			return nil, err
		}
		list = append(list, v)
	}
	return list, rows.Err()
}

// DeleteRuntimeVar deletes a runtime variable of a workspace for an
// environment (0: none active).
func (d *DB) DeleteRuntimeVar(ctx context.Context, workspaceID, envID int64, key string) error {
	return mustAffect(d.db.ExecContext(ctx,
		`DELETE FROM runtime_var WHERE workspace_id = ? AND environment_id = ? AND key = ?`, workspaceID, envID, key))
}

// SetRuntimeVar sets a runtime variable of a workspace for an environment
// (0: none active): scripts write them, they win over every other source.
func (d *DB) SetRuntimeVar(ctx context.Context, workspaceID, envID int64, key, value string) error {
	if !varKey.MatchString(key) {
		return apperr.New(CodeInvalidVarKey).With("key", key)
	}
	_, err := d.db.ExecContext(ctx,
		`INSERT INTO runtime_var (workspace_id, environment_id, key, value) VALUES (?, ?, ?, ?)
		 ON CONFLICT (workspace_id, environment_id, key) DO UPDATE SET value = excluded.value`,
		workspaceID, envID, key, value)
	return err
}
