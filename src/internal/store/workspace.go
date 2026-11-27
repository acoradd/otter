package store

import (
	"context"
	"database/sql"
	"errors"

	"github.com/acoradd/otter/src/internal/apperr"
)

// Workspace is a workspace row, without its network settings.
type Workspace struct {
	ID   int64
	Name string
	// ActiveEnvironmentID is 0 when no environment is active.
	ActiveEnvironmentID int64
}

// Workspaces lists the workspaces by name.
func (d *DB) Workspaces(ctx context.Context) ([]Workspace, error) {
	rows, err := d.db.QueryContext(ctx,
		`SELECT id, name, COALESCE(active_environment_id, 0) FROM workspace ORDER BY name COLLATE NOCASE, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []Workspace{}
	for rows.Next() {
		var w Workspace
		if err := rows.Scan(&w.ID, &w.Name, &w.ActiveEnvironmentID); err != nil {
			return nil, err
		}
		list = append(list, w)
	}
	return list, rows.Err()
}

// Workspace returns a workspace, NOT_FOUND when it does not exist.
func (d *DB) Workspace(ctx context.Context, id int64) (Workspace, error) {
	w := Workspace{ID: id}
	err := d.db.QueryRowContext(ctx,
		`SELECT name, COALESCE(active_environment_id, 0) FROM workspace WHERE id = ?`, id).
		Scan(&w.Name, &w.ActiveEnvironmentID)
	if errors.Is(err, sql.ErrNoRows) {
		return Workspace{}, apperr.New(apperr.NotFound)
	}
	return w, err
}

// CreateWorkspace creates an empty workspace and returns its ID.
func (d *DB) CreateWorkspace(ctx context.Context, name string) (int64, error) {
	name, err := cleanName(name)
	if err != nil {
		return 0, err
	}
	res, err := d.db.ExecContext(ctx, `INSERT INTO workspace (name) VALUES (?)`, name)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// RenameWorkspace renames a workspace.
func (d *DB) RenameWorkspace(ctx context.Context, id int64, name string) error {
	name, err := cleanName(name)
	if err != nil {
		return err
	}
	return mustAffect(d.db.ExecContext(ctx, `UPDATE workspace SET name = ? WHERE id = ?`, name, id))
}

// DeleteWorkspace deletes a workspace with everything in it.
func (d *DB) DeleteWorkspace(ctx context.Context, id int64) error {
	return mustAffect(d.db.ExecContext(ctx, `DELETE FROM workspace WHERE id = ?`, id))
}

// SetActiveEnvironment makes an environment of the workspace the active one,
// or none with envID 0. An environment of another workspace is refused.
func (d *DB) SetActiveEnvironment(ctx context.Context, workspaceID, envID int64) error {
	return d.tx(ctx, func(tx *sql.Tx) error {
		var active any // NULL for none
		if envID != 0 {
			var owner int64
			err := tx.QueryRowContext(ctx, `SELECT workspace_id FROM environment WHERE id = ?`, envID).Scan(&owner)
			if errors.Is(err, sql.ErrNoRows) {
				return apperr.New(apperr.NotFound)
			}
			if err != nil {
				return err
			}
			if owner != workspaceID {
				return apperr.New(apperr.InvalidInput)
			}
			active = envID
		}
		return mustAffect(tx.ExecContext(ctx,
			`UPDATE workspace SET active_environment_id = ? WHERE id = ?`, active, workspaceID))
	})
}
