package store

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/acoradd/otter/src/internal/apperr"
)

var ctx = context.Background()

func open(t *testing.T) *DB {
	t.Helper()
	db, err := Open(ctx, filepath.Join(t.TempDir(), FileName))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func asErr(err error, target **apperr.Error) bool { return errors.As(err, target) }

func wantCode(t *testing.T, err error, code apperr.Code) {
	t.Helper()
	var e *apperr.Error
	if !errors.As(err, &e) || e.Code != code {
		t.Fatalf("error = %v, want %s", err, code)
	}
}

func pragma(t *testing.T, db *DB, name string) string {
	t.Helper()
	var v string
	if err := db.db.QueryRowContext(ctx, "PRAGMA "+name).Scan(&v); err != nil {
		t.Fatal(err)
	}
	return v
}

func TestOpenSetsUpTheDatabase(t *testing.T) {
	// A path with characters a file: URI would mangle, in a folder to create.
	path := filepath.Join(t.TempDir(), "my #1 100% data", "Otter", FileName)
	db, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if _, err := os.Stat(path); err != nil {
		t.Fatalf("the database should be at %s: %v", path, err)
	}
	for name, want := range map[string]string{
		"application_id": "1330926674", // "OTTR"
		"user_version":   "1",
		"journal_mode":   "wal",
		"foreign_keys":   "1",
	} {
		if got := pragma(t, db, name); got != want {
			t.Errorf("PRAGMA %s = %s, want %s", name, got, want)
		}
	}
}

func TestOpenTwiceKeepsTheData(t *testing.T) {
	path := filepath.Join(t.TempDir(), FileName)
	db, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.CreateWorkspace(ctx, "API"); err != nil {
		t.Fatal(err)
	}
	db.Close()

	db, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	list, err := db.Workspaces(ctx)
	if err != nil || len(list) != 1 || list[0].Name != "API" {
		t.Fatalf("workspaces = %v, %v", list, err)
	}
}

func TestOpenRefusesOtherFiles(t *testing.T) {
	dir := t.TempDir()
	stamp := func(name string, pragmas ...string) string {
		path := filepath.Join(dir, name)
		db, err := sql.Open("sqlite", path)
		if err != nil {
			t.Fatal(err)
		}
		defer db.Close()
		for _, p := range pragmas {
			if _, err := db.Exec(p); err != nil {
				t.Fatal(err)
			}
		}
		return path
	}
	notSQLite := filepath.Join(dir, "notes.txt")
	if err := os.WriteFile(notSQLite, []byte("this is not a database, but long enough to have a header......."), 0o644); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		path string
		want apperr.Code
	}{
		{"another app's database", stamp("other.db", "PRAGMA application_id = 42", "CREATE TABLE t (x)"), CodeNotOtterDB},
		{"a newer Otter", stamp("newer.db", "PRAGMA application_id = 1330926674", "PRAGMA user_version = 99"), CodeDBTooNew},
		{"not SQLite", notSQLite, CodeNotOtterDB},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, err := Open(ctx, tt.path)
			if err == nil {
				db.Close()
			}
			wantCode(t, err, tt.want)
		})
	}
}

func TestMigrationsAreApplied(t *testing.T) {
	db := open(t)
	for _, table := range []string{
		"setting", "workspace", "environment", "env_var", "folder", "folder_var", "folder_header",
		"request", "request_var", "request_header", "request_body_field", "runtime_var",
		"import_source", "history", "request_fts", "request_fts_tri",
	} {
		var n int
		if err := db.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE name = ?`, table).Scan(&n); err != nil || n != 1 {
			t.Errorf("table %s: count %d, %v", table, n, err)
		}
	}
}

func TestFullTextIndexesFollowRequests(t *testing.T) {
	db := open(t)
	ws, _ := db.CreateWorkspace(ctx, "API")
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := db.db.ExecContext(ctx, q, args...); err != nil {
			t.Fatal(err)
		}
	}
	count := func(q string, args ...any) int {
		t.Helper()
		var n int
		if err := db.db.QueryRowContext(ctx, q, args...).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	exec(`INSERT INTO folder (id, workspace_id, name) VALUES (1, ?, 'users')`, ws)
	exec(`INSERT INTO request (id, folder_id, name, url) VALUES (1, 1, 'Créer un utilisateur', 'https://api.test/v2/users')`)

	if n := count(`SELECT COUNT(*) FROM request_fts WHERE request_fts MATCH 'creer'`); n != 1 {
		t.Errorf("word search, accents ignored: %d results", n)
	}
	if n := count(`SELECT COUNT(*) FROM request_fts_tri WHERE request_fts_tri MATCH '"v2/use"'`); n != 1 {
		t.Errorf("substring search: %d results", n)
	}

	exec(`UPDATE request SET name = 'Delete account' WHERE id = 1`)
	if n := count(`SELECT COUNT(*) FROM request_fts WHERE request_fts MATCH 'creer'`); n != 0 {
		t.Errorf("old name still found after update: %d", n)
	}
	if n := count(`SELECT COUNT(*) FROM request_fts WHERE request_fts MATCH 'account'`); n != 1 {
		t.Errorf("new name not found after update: %d", n)
	}

	// Deleting the workspace cascades to the request and its index rows.
	if err := db.DeleteWorkspace(ctx, ws); err != nil {
		t.Fatal(err)
	}
	if n := count(`SELECT COUNT(*) FROM request_fts_tri WHERE request_fts_tri MATCH '"v2/use"'`); n != 0 {
		t.Errorf("index rows left after cascade delete: %d", n)
	}
}

func TestSettings(t *testing.T) {
	db := open(t)
	if _, ok, err := db.Setting(ctx, "language"); ok || err != nil {
		t.Fatalf("unset setting: ok=%v err=%v", ok, err)
	}
	for _, v := range []string{"fr", "en"} {
		if err := db.SetSetting(ctx, "language", v); err != nil {
			t.Fatal(err)
		}
		if got, ok, err := db.Setting(ctx, "language"); got != v || !ok || err != nil {
			t.Fatalf("Setting = %q %v %v, want %q", got, ok, err, v)
		}
	}
}
