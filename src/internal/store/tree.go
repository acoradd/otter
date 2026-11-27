package store

import (
	"context"
	"database/sql"
	"errors"
	"sort"
	"strings"

	"github.com/acoradd/otter/src/internal/apperr"
)

// CodeFolderCycle is a folder moved into itself or one of its descendants.
const CodeFolderCycle apperr.Code = "FOLDER_CYCLE"

// NodeKind tells a folder from a request in the tree.
type NodeKind string

const (
	KindFolder  NodeKind = "folder"
	KindRequest NodeKind = "request"
)

// Node is a row of the tree: a folder or a request.
type Node struct {
	Kind NodeKind
	ID   int64
	Name string
	// Method is the HTTP method of a request, "" for a folder.
	Method string
	// HasChildren: a folder that holds folders or requests.
	HasChildren bool
}

// Folder is a folder row.
type Folder struct {
	ID          int64
	WorkspaceID int64
	// ParentID is 0 for a folder at the root of the workspace.
	ParentID int64
	Name     string
}

// RequestRef locates a request in the tree.
type RequestRef struct {
	ID          int64
	FolderID    int64
	WorkspaceID int64
	Name        string
	Method      string
}

// FolderPath is a folder with the names from the root down to it.
type FolderPath struct {
	ID   int64
	Path []string
}

// Children returns the direct children of a folder, or of the workspace
// root with folderID 0: folders first, then requests, each in their order.
// Requests always live in a folder: the root holds folders only.
func (d *DB) Children(ctx context.Context, workspaceID, folderID int64) ([]Node, error) {
	nodes := []Node{}
	const folders = `SELECT f.id, f.name,
	  EXISTS (SELECT 1 FROM folder c WHERE c.parent_id = f.id) OR EXISTS (SELECT 1 FROM request r WHERE r.folder_id = f.id)
	  FROM folder f WHERE f.workspace_id = ? AND f.parent_id IS ? ORDER BY f.seq, f.id`
	var parent any
	if folderID != 0 {
		parent = folderID
	}
	rows, err := d.db.QueryContext(ctx, folders, workspaceID, parent)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		n := Node{Kind: KindFolder}
		if err := rows.Scan(&n.ID, &n.Name, &n.HasChildren); err != nil {
			rows.Close()
			return nil, err
		}
		nodes = append(nodes, n)
	}
	rows.Close()
	if err := rows.Err(); err != nil || folderID == 0 {
		return nodes, err
	}
	rows, err = d.db.QueryContext(ctx,
		`SELECT id, name, method FROM request WHERE folder_id = ? ORDER BY seq, id`, folderID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		n := Node{Kind: KindRequest}
		if err := rows.Scan(&n.ID, &n.Name, &n.Method); err != nil {
			return nil, err
		}
		nodes = append(nodes, n)
	}
	return nodes, rows.Err()
}

// Folder returns a folder, NOT_FOUND when it does not exist.
func (d *DB) Folder(ctx context.Context, id int64) (Folder, error) {
	return folderRow(ctx, d.db, id)
}

type rowQuerier interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func folderRow(ctx context.Context, q rowQuerier, id int64) (Folder, error) {
	f := Folder{ID: id}
	err := q.QueryRowContext(ctx, `SELECT workspace_id, COALESCE(parent_id, 0), name FROM folder WHERE id = ?`, id).
		Scan(&f.WorkspaceID, &f.ParentID, &f.Name)
	if errors.Is(err, sql.ErrNoRows) {
		return Folder{}, apperr.New(apperr.NotFound)
	}
	return f, err
}

// Request returns where a request is, NOT_FOUND when it does not exist.
func (d *DB) Request(ctx context.Context, id int64) (RequestRef, error) {
	return requestRow(ctx, d.db, id)
}

func requestRow(ctx context.Context, q rowQuerier, id int64) (RequestRef, error) {
	r := RequestRef{ID: id}
	err := q.QueryRowContext(ctx,
		`SELECT r.folder_id, f.workspace_id, r.name, r.method FROM request r JOIN folder f ON f.id = r.folder_id WHERE r.id = ?`, id).
		Scan(&r.FolderID, &r.WorkspaceID, &r.Name, &r.Method)
	if errors.Is(err, sql.ErrNoRows) {
		return RequestRef{}, apperr.New(apperr.NotFound)
	}
	return r, err
}

// FolderPaths lists every folder of a workspace with its path, sorted by
// path: the choices of "Move to".
func (d *DB) FolderPaths(ctx context.Context, workspaceID int64) ([]FolderPath, error) {
	rows, err := d.db.QueryContext(ctx,
		`SELECT id, COALESCE(parent_id, 0), name FROM folder WHERE workspace_id = ?`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	type row struct {
		parent int64
		name   string
	}
	all := map[int64]row{}
	for rows.Next() {
		var id int64
		var r row
		if err := rows.Scan(&id, &r.parent, &r.name); err != nil {
			return nil, err
		}
		all[id] = r
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	paths := make([]FolderPath, 0, len(all))
	for id := range all {
		var path []string
		for cur, n := id, 0; cur != 0 && n <= len(all); n++ {
			r := all[cur]
			path = append([]string{r.name}, path...)
			cur = r.parent
		}
		paths = append(paths, FolderPath{ID: id, Path: path})
	}
	sort.Slice(paths, func(i, j int) bool {
		a, b := paths[i].Path, paths[j].Path
		for k := 0; k < len(a) && k < len(b); k++ {
			if x, y := strings.ToLower(a[k]), strings.ToLower(b[k]); x != y {
				return x < y
			}
		}
		if len(a) != len(b) {
			return len(a) < len(b)
		}
		return paths[i].ID < paths[j].ID
	})
	return paths, nil
}

// CreateFolder adds a folder at the end of a folder (or of the workspace
// root with parentID 0) and returns its ID.
func (d *DB) CreateFolder(ctx context.Context, workspaceID, parentID int64, name string) (int64, error) {
	name, err := cleanName(name)
	if err != nil {
		return 0, err
	}
	var id int64
	err = d.tx(ctx, func(tx *sql.Tx) error {
		if err := exists(ctx, tx, `SELECT 1 FROM workspace WHERE id = ?`, workspaceID); err != nil {
			return err
		}
		if err := checkParent(ctx, tx, workspaceID, parentID); err != nil {
			return err
		}
		res, err := tx.ExecContext(ctx,
			`INSERT INTO folder (workspace_id, parent_id, name, seq)
			 VALUES (?, ?, ?, (SELECT COALESCE(MAX(seq), -1) + 1 FROM folder WHERE workspace_id = ? AND parent_id IS ?))`,
			workspaceID, nullID(parentID), name, workspaceID, nullID(parentID))
		if err != nil {
			return err
		}
		id, err = res.LastInsertId()
		return err
	})
	return id, err
}

// CreateRequest adds a GET request with no URL at the end of a folder and
// returns its ID.
func (d *DB) CreateRequest(ctx context.Context, folderID int64, name string) (int64, error) {
	name, err := cleanName(name)
	if err != nil {
		return 0, err
	}
	var id int64
	err = d.tx(ctx, func(tx *sql.Tx) error {
		if err := exists(ctx, tx, `SELECT 1 FROM folder WHERE id = ?`, folderID); err != nil {
			return err
		}
		res, err := tx.ExecContext(ctx,
			`INSERT INTO request (folder_id, name, seq)
			 VALUES (?, ?, (SELECT COALESCE(MAX(seq), -1) + 1 FROM request WHERE folder_id = ?))`,
			folderID, name, folderID)
		if err != nil {
			return err
		}
		id, err = res.LastInsertId()
		return err
	})
	return id, err
}

// RenameFolder renames a folder.
func (d *DB) RenameFolder(ctx context.Context, id int64, name string) error {
	name, err := cleanName(name)
	if err != nil {
		return err
	}
	return mustAffect(d.db.ExecContext(ctx, `UPDATE folder SET name = ? WHERE id = ?`, name, id))
}

// RenameRequest renames a request.
func (d *DB) RenameRequest(ctx context.Context, id int64, name string) error {
	name, err := cleanName(name)
	if err != nil {
		return err
	}
	return mustAffect(d.db.ExecContext(ctx, `UPDATE request SET name = ? WHERE id = ?`, name, id))
}

// DeleteFolder deletes a folder with everything in it.
func (d *DB) DeleteFolder(ctx context.Context, id int64) error {
	return mustAffect(d.db.ExecContext(ctx, `DELETE FROM folder WHERE id = ?`, id))
}

// DeleteRequest deletes a request.
func (d *DB) DeleteRequest(ctx context.Context, id int64) error {
	return mustAffect(d.db.ExecContext(ctx, `DELETE FROM request WHERE id = ?`, id))
}

// MoveFolder moves a folder into parentID (0: the workspace root), at index
// among the folders there (-1 or past the end: last). A folder never leaves
// its workspace, nor goes into itself or a descendant (FOLDER_CYCLE).
func (d *DB) MoveFolder(ctx context.Context, id, parentID int64, index int) error {
	return d.tx(ctx, func(tx *sql.Tx) error {
		f, err := folderRow(ctx, tx, id)
		if err != nil {
			return err
		}
		if err := checkParent(ctx, tx, f.WorkspaceID, parentID); err != nil {
			return err
		}
		if parentID != 0 {
			var cycle bool
			err := tx.QueryRowContext(ctx, `WITH RECURSIVE up(id, parent_id) AS (
			    SELECT id, parent_id FROM folder WHERE id = ?
			    UNION ALL
			    SELECT f.id, f.parent_id FROM folder f JOIN up ON f.id = up.parent_id
			  ) SELECT EXISTS (SELECT 1 FROM up WHERE id = ?)`, parentID, id).Scan(&cycle)
			if err != nil {
				return err
			}
			if cycle {
				return apperr.New(CodeFolderCycle)
			}
		}
		if _, err := tx.ExecContext(ctx, `UPDATE folder SET parent_id = ? WHERE id = ?`, nullID(parentID), id); err != nil {
			return err
		}
		siblings, err := ids(ctx, tx,
			`SELECT id FROM folder WHERE workspace_id = ? AND parent_id IS ? AND id <> ? ORDER BY seq, id`,
			f.WorkspaceID, nullID(parentID), id)
		if err != nil {
			return err
		}
		return renumber(ctx, tx, "folder", insertAt(siblings, id, index))
	})
}

// MoveRequest moves a request into a folder of the same workspace, at index
// among the requests there (-1 or past the end: last).
func (d *DB) MoveRequest(ctx context.Context, id, folderID int64, index int) error {
	return d.tx(ctx, func(tx *sql.Tx) error {
		r, err := requestRow(ctx, tx, id)
		if err != nil {
			return err
		}
		if err := checkParent(ctx, tx, r.WorkspaceID, folderID); err != nil {
			return err
		}
		if folderID == 0 {
			return apperr.New(apperr.InvalidInput) // requests live in a folder
		}
		if _, err := tx.ExecContext(ctx, `UPDATE request SET folder_id = ? WHERE id = ?`, folderID, id); err != nil {
			return err
		}
		siblings, err := ids(ctx, tx, `SELECT id FROM request WHERE folder_id = ? AND id <> ? ORDER BY seq, id`, folderID, id)
		if err != nil {
			return err
		}
		return renumber(ctx, tx, "request", insertAt(siblings, id, index))
	})
}

// DuplicateFolder copies a folder with everything in it (variables, headers,
// subfolders, requests), names the copy, places it right after the original
// and returns its ID.
func (d *DB) DuplicateFolder(ctx context.Context, id int64, name string) (int64, error) {
	name, err := cleanName(name)
	if err != nil {
		return 0, err
	}
	var copyID int64
	err = d.tx(ctx, func(tx *sql.Tx) error {
		f, err := folderRow(ctx, tx, id)
		if err != nil {
			return err
		}
		if copyID, err = copyFolder(ctx, tx, id, f.ParentID, name); err != nil {
			return err
		}
		siblings, err := ids(ctx, tx,
			`SELECT id FROM folder WHERE workspace_id = ? AND parent_id IS ? AND id <> ? ORDER BY seq, id`,
			f.WorkspaceID, nullID(f.ParentID), copyID)
		if err != nil {
			return err
		}
		return renumber(ctx, tx, "folder", insertAfter(siblings, copyID, id))
	})
	return copyID, err
}

// DuplicateRequest copies a request (variables, headers, body fields), names
// the copy, places it right after the original and returns its ID.
func (d *DB) DuplicateRequest(ctx context.Context, id int64, name string) (int64, error) {
	name, err := cleanName(name)
	if err != nil {
		return 0, err
	}
	var copyID int64
	err = d.tx(ctx, func(tx *sql.Tx) error {
		r, err := requestRow(ctx, tx, id)
		if err != nil {
			return err
		}
		if copyID, err = copyRequest(ctx, tx, id, r.FolderID, name); err != nil {
			return err
		}
		siblings, err := ids(ctx, tx, `SELECT id FROM request WHERE folder_id = ? AND id <> ? ORDER BY seq, id`, r.FolderID, copyID)
		if err != nil {
			return err
		}
		return renumber(ctx, tx, "request", insertAfter(siblings, copyID, id))
	})
	return copyID, err
}

// copyFolder copies a folder into parentID, keeping its seq, with its
// variables, headers, requests and subfolders. name "" keeps the name.
func copyFolder(ctx context.Context, tx *sql.Tx, id, parentID int64, name string) (int64, error) {
	res, err := tx.ExecContext(ctx,
		`INSERT INTO folder (workspace_id, parent_id, name, seq)
		 SELECT workspace_id, ?, CASE WHEN ? = '' THEN name ELSE ? END, seq FROM folder WHERE id = ?`,
		nullID(parentID), name, name, id)
	if err != nil {
		return 0, err
	}
	copyID, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	for _, q := range []string{
		`INSERT INTO folder_var (folder_id, seq, key, value, enabled)
		 SELECT ?, seq, key, value, enabled FROM folder_var WHERE folder_id = ?`,
		`INSERT INTO folder_header (folder_id, seq, key, value, mode, enabled)
		 SELECT ?, seq, key, value, mode, enabled FROM folder_header WHERE folder_id = ?`,
	} {
		if _, err := tx.ExecContext(ctx, q, copyID, id); err != nil {
			return 0, err
		}
	}
	requests, err := ids(ctx, tx, `SELECT id FROM request WHERE folder_id = ? ORDER BY seq, id`, id)
	if err != nil {
		return 0, err
	}
	for _, r := range requests {
		if _, err := copyRequest(ctx, tx, r, copyID, ""); err != nil {
			return 0, err
		}
	}
	folders, err := ids(ctx, tx, `SELECT id FROM folder WHERE parent_id = ? ORDER BY seq, id`, id)
	if err != nil {
		return 0, err
	}
	for _, f := range folders {
		if _, err := copyFolder(ctx, tx, f, copyID, ""); err != nil {
			return 0, err
		}
	}
	return copyID, nil
}

// copyRequest copies a request into folderID, keeping its seq, with its
// variables, headers and body fields. name "" keeps the name.
func copyRequest(ctx context.Context, tx *sql.Tx, id, folderID int64, name string) (int64, error) {
	res, err := tx.ExecContext(ctx,
		`INSERT INTO request (folder_id, seq, name, method, url, body_type, body, body_file, post_script,
		   encode_url, follow_redirects, max_redirects, timeout_ms)
		 SELECT ?, seq, CASE WHEN ? = '' THEN name ELSE ? END, method, url, body_type, body, body_file, post_script,
		   encode_url, follow_redirects, max_redirects, timeout_ms
		 FROM request WHERE id = ?`, folderID, name, name, id)
	if err != nil {
		return 0, err
	}
	copyID, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	for _, q := range []string{
		`INSERT INTO request_var (request_id, seq, key, value, enabled)
		 SELECT ?, seq, key, value, enabled FROM request_var WHERE request_id = ?`,
		`INSERT INTO request_header (request_id, seq, key, value, mode, enabled)
		 SELECT ?, seq, key, value, mode, enabled FROM request_header WHERE request_id = ?`,
		`INSERT INTO request_body_field (request_id, seq, key, kind, value, content_type, enabled)
		 SELECT ?, seq, key, kind, value, content_type, enabled FROM request_body_field WHERE request_id = ?`,
	} {
		if _, err := tx.ExecContext(ctx, q, copyID, id); err != nil {
			return 0, err
		}
	}
	return copyID, nil
}

// checkParent checks a parent folder exists in the workspace (0: the root).
func checkParent(ctx context.Context, tx *sql.Tx, workspaceID, parentID int64) error {
	if parentID == 0 {
		return nil
	}
	p, err := folderRow(ctx, tx, parentID)
	if err != nil {
		return err
	}
	if p.WorkspaceID != workspaceID {
		return apperr.New(apperr.InvalidInput) // never across workspaces
	}
	return nil
}

func nullID(id int64) any {
	if id == 0 {
		return nil
	}
	return id
}

func ids(ctx context.Context, tx *sql.Tx, query string, args ...any) ([]int64, error) {
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		list = append(list, id)
	}
	return list, rows.Err()
}

// insertAt puts id at index in list (-1 or past the end: last).
func insertAt(list []int64, id int64, index int) []int64 {
	if index < 0 || index > len(list) {
		index = len(list)
	}
	out := make([]int64, 0, len(list)+1)
	out = append(out, list[:index]...)
	out = append(out, id)
	return append(out, list[index:]...)
}

// insertAfter puts id right after after in list (last when after is absent).
func insertAfter(list []int64, id, after int64) []int64 {
	for i, v := range list {
		if v == after {
			return insertAt(list, id, i+1)
		}
	}
	return insertAt(list, id, -1)
}

// renumber writes seq 0..n-1 in the order of ids. table is a constant.
func renumber(ctx context.Context, tx *sql.Tx, table string, order []int64) error {
	stmt, err := tx.PrepareContext(ctx, `UPDATE `+table+` SET seq = ? WHERE id = ? AND seq <> ?`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for i, id := range order {
		if _, err := stmt.ExecContext(ctx, i, id, i); err != nil {
			return err
		}
	}
	return nil
}

// FolderPathOf returns the names from the root down to a folder.
func (d *DB) FolderPathOf(ctx context.Context, id int64) ([]string, error) {
	rows, err := d.db.QueryContext(ctx, `WITH RECURSIVE up(id, parent_id, name, depth) AS (
	    SELECT id, parent_id, name, 0 FROM folder WHERE id = ?
	    UNION ALL
	    SELECT f.id, f.parent_id, f.name, up.depth + 1 FROM folder f JOIN up ON f.id = up.parent_id
	  ) SELECT name FROM up ORDER BY depth DESC`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	path := []string{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		path = append(path, name)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(path) == 0 {
		return nil, apperr.New(apperr.NotFound)
	}
	return path, nil
}
