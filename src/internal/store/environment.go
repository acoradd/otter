package store

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"strings"

	"github.com/acoradd/otter/src/internal/apperr"
)

// Environment is an environment of a workspace.
type Environment struct {
	ID          int64
	WorkspaceID int64
	Name        string
}

// EnvVar is a variable of an environment, in its order.
//
// Secret values are stored as is until the secrets step: they will be
// encrypted with AES-GCM (docs/specs/scope-v1.md, section 11).
type EnvVar struct {
	Key     string
	Value   string
	Secret  bool
	Enabled bool
}

// varKey is the variable name syntax of {{name}} (scope-v1.md, section 4).
var varKey = regexp.MustCompile(`^[\w.\-]+$`)

// Environments lists the environments of a workspace in their order.
func (d *DB) Environments(ctx context.Context, workspaceID int64) ([]Environment, error) {
	rows, err := d.db.QueryContext(ctx,
		`SELECT id, workspace_id, name FROM environment WHERE workspace_id = ? ORDER BY seq, id`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []Environment{}
	for rows.Next() {
		var e Environment
		if err := rows.Scan(&e.ID, &e.WorkspaceID, &e.Name); err != nil {
			return nil, err
		}
		list = append(list, e)
	}
	return list, rows.Err()
}

// Environment returns an environment, NOT_FOUND when it does not exist.
func (d *DB) Environment(ctx context.Context, id int64) (Environment, error) {
	e := Environment{ID: id}
	err := d.db.QueryRowContext(ctx, `SELECT workspace_id, name FROM environment WHERE id = ?`, id).
		Scan(&e.WorkspaceID, &e.Name)
	if errors.Is(err, sql.ErrNoRows) {
		return Environment{}, apperr.New(apperr.NotFound)
	}
	return e, err
}

// CreateEnvironment adds an empty environment at the end of the workspace's
// list and returns its ID.
func (d *DB) CreateEnvironment(ctx context.Context, workspaceID int64, name string) (int64, error) {
	name, err := cleanName(name)
	if err != nil {
		return 0, err
	}
	var id int64
	err = d.tx(ctx, func(tx *sql.Tx) error {
		if err := exists(ctx, tx, `SELECT 1 FROM workspace WHERE id = ?`, workspaceID); err != nil {
			return err
		}
		res, err := tx.ExecContext(ctx,
			`INSERT INTO environment (workspace_id, name, seq)
			 VALUES (?, ?, (SELECT COALESCE(MAX(seq), -1) + 1 FROM environment WHERE workspace_id = ?))`,
			workspaceID, name, workspaceID)
		if err != nil {
			return err
		}
		id, err = res.LastInsertId()
		return err
	})
	return id, err
}

// RenameEnvironment renames an environment.
func (d *DB) RenameEnvironment(ctx context.Context, id int64, name string) error {
	name, err := cleanName(name)
	if err != nil {
		return err
	}
	return mustAffect(d.db.ExecContext(ctx, `UPDATE environment SET name = ? WHERE id = ?`, name, id))
}

// DeleteEnvironment deletes an environment and its variables. A workspace
// where it was active is left without an active environment.
func (d *DB) DeleteEnvironment(ctx context.Context, id int64) error {
	return mustAffect(d.db.ExecContext(ctx, `DELETE FROM environment WHERE id = ?`, id))
}

// EnvVars returns the variables of an environment in their order.
func (d *DB) EnvVars(ctx context.Context, envID int64) ([]EnvVar, error) {
	rows, err := d.db.QueryContext(ctx,
		`SELECT key, value, secret, enabled FROM env_var WHERE environment_id = ? ORDER BY seq, id`, envID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []EnvVar{}
	for rows.Next() {
		var v EnvVar
		if err := rows.Scan(&v.Key, &v.Value, &v.Secret, &v.Enabled); err != nil {
			return nil, err
		}
		list = append(list, v)
	}
	return list, rows.Err()
}

// SetEnvVars replaces the variables of an environment, in the given order.
// Keys are trimmed; rows with neither key nor value are dropped; a key
// outside [\w.-]+ is refused with INVALID_VAR_KEY (param key).
func (d *DB) SetEnvVars(ctx context.Context, envID int64, vars []EnvVar) error {
	clean := make([]EnvVar, 0, len(vars))
	for _, v := range vars {
		v.Key = strings.TrimSpace(v.Key)
		if v.Key == "" && v.Value == "" {
			continue
		}
		if !varKey.MatchString(v.Key) {
			return apperr.New(CodeInvalidVarKey).With("key", v.Key)
		}
		clean = append(clean, v)
	}
	return d.tx(ctx, func(tx *sql.Tx) error {
		if err := exists(ctx, tx, `SELECT 1 FROM environment WHERE id = ?`, envID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM env_var WHERE environment_id = ?`, envID); err != nil {
			return err
		}
		stmt, err := tx.PrepareContext(ctx,
			`INSERT INTO env_var (environment_id, seq, key, value, secret, enabled) VALUES (?, ?, ?, ?, ?, ?)`)
		if err != nil {
			return err
		}
		defer stmt.Close()
		for i, v := range clean {
			if _, err := stmt.ExecContext(ctx, envID, i, v.Key, v.Value, v.Secret, v.Enabled); err != nil {
				return err
			}
		}
		return nil
	})
}

// exists returns NOT_FOUND when the query finds no row.
func exists(ctx context.Context, tx *sql.Tx, query string, args ...any) error {
	var one int
	err := tx.QueryRowContext(ctx, query, args...).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return apperr.New(apperr.NotFound)
	}
	return err
}
