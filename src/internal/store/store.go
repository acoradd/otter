// Package store is the SQLite access of Otter: opening otter.db (WAL,
// foreign keys, application_id OTTR), append-only migrations versioned by
// user_version, queries and full-text search (FTS5).
//
// Every change comes with tests.
package store

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite" // registers the "sqlite" driver

	"github.com/acoradd/otter/src/internal/apperr"
)

// FileName is the name of the database in the app's data folder.
const FileName = "otter.db"

// appID is the SQLite application_id of otter.db, "OTTR", stored in the
// file header so the file is recognized whatever its name.
const appID int32 = 0x4F545452

const (
	// CodeNotOtterDB is a file that is not an Otter database.
	CodeNotOtterDB apperr.Code = "DB_NOT_OTTER"
	// CodeDBTooNew is a database written by a newer Otter.
	CodeDBTooNew apperr.Code = "DB_TOO_NEW"
	// CodeNameRequired is an empty workspace or environment name.
	CodeNameRequired apperr.Code = "NAME_REQUIRED"
	// CodeInvalidVarKey is a variable name outside [\w.-]+.
	CodeInvalidVarKey apperr.Code = "INVALID_VAR_KEY"
)

// DB is otter.db.
type DB struct {
	db *sql.DB
}

// Open opens the database at path, creating it and its folder if needed,
// and migrates it to the latest schema.
func Open(ctx context.Context, path string) (*DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	q := strings.Join([]string{
		"_pragma=journal_mode(WAL)",
		"_pragma=synchronous(NORMAL)",
		"_pragma=foreign_keys(1)",
		"_pragma=busy_timeout(5000)",
		"_txlock=immediate",
	}, "&")
	// No "file:" prefix: the path is passed as is, not parsed as a URI, so
	// Windows paths and names with '#' or '%' work.
	db, err := sql.Open("sqlite", path+"?"+q)
	if err != nil {
		return nil, err
	}
	// One writer: a single connection keeps writes ordered and transactions
	// simple. Calls are in-process and short.
	db.SetMaxOpenConns(1)
	if err := migrate(ctx, db, migrations); err != nil {
		db.Close()
		return nil, err
	}
	return &DB{db: db}, nil
}

// Close closes the database.
func (d *DB) Close() error { return d.db.Close() }

// migration upgrades the schema by one version, inside a transaction.
type migration func(ctx context.Context, tx *sql.Tx) error

// migrate checks the file is an Otter database (or a new, empty one, then
// stamped), and applies the migrations it lacks, in one transaction.
func migrate(ctx context.Context, db *sql.DB, list []migration) error {
	var id int32
	var version int
	if err := db.QueryRowContext(ctx, "PRAGMA application_id").Scan(&id); err != nil {
		return apperr.Wrap(CodeNotOtterDB, err) // not a SQLite file at all
	}
	if err := db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return err
	}
	switch {
	case id == 0 && version == 0:
		if _, err := db.ExecContext(ctx, fmt.Sprintf("PRAGMA application_id = %d", appID)); err != nil {
			return err
		}
	case id != appID:
		return apperr.New(CodeNotOtterDB)
	}
	if version > len(list) {
		return apperr.New(CodeDBTooNew)
	}
	if version == len(list) {
		return nil
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for i := version; i < len(list); i++ {
		if err := list[i](ctx, tx); err != nil {
			return fmt.Errorf("migration %d: %w", i+1, err)
		}
	}
	if _, err := tx.ExecContext(ctx, fmt.Sprintf("PRAGMA user_version = %d", len(list))); err != nil {
		return err
	}
	return tx.Commit()
}

func execAll(statements ...string) migration {
	return func(ctx context.Context, tx *sql.Tx) error {
		for _, s := range statements {
			if _, err := tx.ExecContext(ctx, s); err != nil {
				return err
			}
		}
		return nil
	}
}

// tx runs fn in a write transaction.
func (d *DB) tx(ctx context.Context, fn func(tx *sql.Tx) error) error {
	t, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer t.Rollback()
	if err := fn(t); err != nil {
		return err
	}
	return t.Commit()
}

// cleanName trims a name and refuses an empty one.
func cleanName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", apperr.New(CodeNameRequired)
	}
	return name, nil
}

// mustAffect turns "no row changed" into NOT_FOUND.
func mustAffect(res sql.Result, err error) error {
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return apperr.New(apperr.NotFound)
	}
	return nil
}
