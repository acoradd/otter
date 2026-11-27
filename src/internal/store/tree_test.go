package store

import (
	"reflect"
	"strings"
	"testing"

	"github.com/acoradd/otter/src/internal/apperr"
)

// treeFixture builds:
//
//	API
//	├── users (folder) ── list, create (requests), admin (folder) ── purge
//	└── orders (folder)
type treeFixture struct {
	db                              *DB
	ws, other                       int64
	users, admin, orders, foreign   int64
	list, create, purge, foreignReq int64
}

func newTree(t *testing.T) treeFixture {
	t.Helper()
	db := open(t)
	f := treeFixture{db: db}
	f.ws, _ = db.CreateWorkspace(ctx, "API")
	f.other, _ = db.CreateWorkspace(ctx, "Other")
	f.users = must(t)(db.CreateFolder(ctx, f.ws, 0, "users"))
	f.orders = must(t)(db.CreateFolder(ctx, f.ws, 0, "orders"))
	f.admin = must(t)(db.CreateFolder(ctx, f.ws, f.users, "admin"))
	f.list = must(t)(db.CreateRequest(ctx, f.users, "list"))
	f.create = must(t)(db.CreateRequest(ctx, f.users, "create"))
	f.purge = must(t)(db.CreateRequest(ctx, f.admin, "purge"))
	f.foreign = must(t)(db.CreateFolder(ctx, f.other, 0, "elsewhere"))
	f.foreignReq = must(t)(db.CreateRequest(ctx, f.foreign, "far"))
	return f
}

func must(t *testing.T) func(int64, error) int64 {
	return func(id int64, err error) int64 {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
}

// names lists the children of a folder as "kind:name".
func (f treeFixture) names(t *testing.T, folder int64) string {
	t.Helper()
	nodes, err := f.db.Children(ctx, f.ws, folder)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, n := range nodes {
		out = append(out, string(n.Kind[0])+":"+n.Name)
	}
	return strings.Join(out, " ")
}

func TestChildren(t *testing.T) {
	f := newTree(t)
	root, err := f.db.Children(ctx, f.ws, 0)
	if err != nil {
		t.Fatal(err)
	}
	want := []Node{
		{Kind: KindFolder, ID: f.users, Name: "users", HasChildren: true},
		{Kind: KindFolder, ID: f.orders, Name: "orders", HasChildren: false},
	}
	if !reflect.DeepEqual(root, want) {
		t.Fatalf("root = %+v, want %+v", root, want)
	}
	users, _ := f.db.Children(ctx, f.ws, f.users)
	wantUsers := []Node{
		{Kind: KindFolder, ID: f.admin, Name: "admin", HasChildren: true},
		{Kind: KindRequest, ID: f.list, Name: "list", Method: "GET"},
		{Kind: KindRequest, ID: f.create, Name: "create", Method: "GET"},
	}
	if !reflect.DeepEqual(users, wantUsers) {
		t.Fatalf("users = %+v, want %+v (folders first, then requests)", users, wantUsers)
	}
}

func TestTreeErrors(t *testing.T) {
	f := newTree(t)
	db := f.db
	tests := []struct {
		name string
		err  error
		want apperr.Code
	}{
		{"folder without a name", second(db.CreateFolder(ctx, f.ws, 0, " ")), CodeNameRequired},
		{"folder in a missing workspace", second(db.CreateFolder(ctx, 999, 0, "x")), apperr.NotFound},
		{"folder in a missing parent", second(db.CreateFolder(ctx, f.ws, 999, "x")), apperr.NotFound},
		{"folder in another workspace's folder", second(db.CreateFolder(ctx, f.ws, f.foreign, "x")), apperr.InvalidInput},
		{"request in a missing folder", second(db.CreateRequest(ctx, 999, "x")), apperr.NotFound},
		{"request without a name", second(db.CreateRequest(ctx, f.users, "")), CodeNameRequired},
		{"rename a missing folder", db.RenameFolder(ctx, 999, "x"), apperr.NotFound},
		{"rename a request to nothing", db.RenameRequest(ctx, f.list, " "), CodeNameRequired},
		{"delete a missing request", db.DeleteRequest(ctx, 999), apperr.NotFound},
		{"delete a missing folder", db.DeleteFolder(ctx, 999), apperr.NotFound},
		{"folder into itself", db.MoveFolder(ctx, f.users, f.users, 0), CodeFolderCycle},
		{"folder into its child", db.MoveFolder(ctx, f.users, f.admin, 0), CodeFolderCycle},
		{"folder into another workspace", db.MoveFolder(ctx, f.users, f.foreign, 0), apperr.InvalidInput},
		{"request into another workspace", db.MoveRequest(ctx, f.list, f.foreign, 0), apperr.InvalidInput},
		{"request to the root", db.MoveRequest(ctx, f.list, 0, 0), apperr.InvalidInput},
		{"move a missing request", db.MoveRequest(ctx, 999, f.users, 0), apperr.NotFound},
		{"duplicate a missing folder", second(db.DuplicateFolder(ctx, 999, "x")), apperr.NotFound},
		{"duplicate without a name", second(db.DuplicateRequest(ctx, f.list, "")), CodeNameRequired},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) { wantCode(t, tt.err, tt.want) })
	}
	if got := f.names(t, 0); got != "f:users f:orders" {
		t.Errorf("refused changes changed the root: %s", got)
	}
}

func second(_ int64, err error) error { return err }

func TestMoveFolder(t *testing.T) {
	tests := []struct {
		name     string
		move     func(f treeFixture) error
		root     string
		users    string
		orders   string
		parentOf func(f treeFixture) (id, parent int64)
	}{
		{
			name:   "reorder at the root",
			move:   func(f treeFixture) error { return f.db.MoveFolder(ctx, f.orders, 0, 0) },
			root:   "f:orders f:users",
			users:  "f:admin r:list r:create",
			orders: "",
		},
		{
			name:   "into another folder, last",
			move:   func(f treeFixture) error { return f.db.MoveFolder(ctx, f.admin, f.orders, -1) },
			root:   "f:users f:orders",
			users:  "r:list r:create",
			orders: "f:admin",
		},
		{
			name:   "a deep folder to the root, first",
			move:   func(f treeFixture) error { return f.db.MoveFolder(ctx, f.admin, 0, 0) },
			root:   "f:admin f:users f:orders",
			users:  "r:list r:create",
			orders: "",
		},
		{
			name:   "an index past the end is last",
			move:   func(f treeFixture) error { return f.db.MoveFolder(ctx, f.users, 0, 99) },
			root:   "f:orders f:users",
			users:  "f:admin r:list r:create",
			orders: "",
		},
		{
			name:   "a parent into a sibling: not a cycle",
			move:   func(f treeFixture) error { return f.db.MoveFolder(ctx, f.orders, f.users, 1) },
			root:   "f:users",
			users:  "f:admin f:orders r:list r:create",
			orders: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newTree(t)
			if err := tt.move(f); err != nil {
				t.Fatal(err)
			}
			if got := f.names(t, 0); got != tt.root {
				t.Errorf("root = %q, want %q", got, tt.root)
			}
			if got := f.names(t, f.users); got != tt.users {
				t.Errorf("users = %q, want %q", got, tt.users)
			}
			if got := f.names(t, f.orders); got != tt.orders {
				t.Errorf("orders = %q, want %q", got, tt.orders)
			}
		})
	}
}

func TestMoveRequest(t *testing.T) {
	tests := []struct {
		name   string
		move   func(f treeFixture) error
		users  string
		orders string
	}{
		{"reorder", func(f treeFixture) error { return f.db.MoveRequest(ctx, f.create, f.users, 0) }, "f:admin r:create r:list", ""},
		{"same place", func(f treeFixture) error { return f.db.MoveRequest(ctx, f.list, f.users, 0) }, "f:admin r:list r:create", ""},
		{"to the end", func(f treeFixture) error { return f.db.MoveRequest(ctx, f.list, f.users, -1) }, "f:admin r:create r:list", ""},
		{"to another folder", func(f treeFixture) error { return f.db.MoveRequest(ctx, f.list, f.orders, 0) }, "f:admin r:create", "r:list"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newTree(t)
			if err := tt.move(f); err != nil {
				t.Fatal(err)
			}
			if got := f.names(t, f.users); got != tt.users {
				t.Errorf("users = %q, want %q", got, tt.users)
			}
			if got := f.names(t, f.orders); got != tt.orders {
				t.Errorf("orders = %q, want %q", got, tt.orders)
			}
		})
	}
}

func TestDuplicate(t *testing.T) {
	f := newTree(t)
	db := f.db
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := db.db.ExecContext(ctx, q, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(`UPDATE request SET method = 'POST', url = '{{baseUrl}}/users', body_type = 'json', body = '{}' WHERE id = ?`, f.create)
	exec(`INSERT INTO request_header (request_id, key, value) VALUES (?, 'X-Trace', '1')`, f.create)
	exec(`INSERT INTO folder_var (folder_id, key, value) VALUES (?, 'scope', 'admin')`, f.admin)
	exec(`INSERT INTO folder_header (folder_id, key, mode) VALUES (?, 'Authorization', 'ignore')`, f.admin)

	reqCopy, err := db.DuplicateRequest(ctx, f.list, "list (copy)")
	if err != nil {
		t.Fatal(err)
	}
	if got := f.names(t, f.users); got != "f:admin r:list r:list (copy) r:create" {
		t.Fatalf("users = %q: the copy goes right after the original", got)
	}

	usersCopy, err := db.DuplicateFolder(ctx, f.users, "users (copy)")
	if err != nil {
		t.Fatal(err)
	}
	if got := f.names(t, 0); got != "f:users f:users (copy) f:orders" {
		t.Fatalf("root = %q", got)
	}
	if got := f.names(t, usersCopy); got != "f:admin r:list r:list (copy) r:create" {
		t.Fatalf("copied folder = %q: children keep their names and order", got)
	}

	// The deep copy is independent and complete.
	nodes, _ := db.Children(ctx, f.ws, usersCopy)
	adminCopy, createCopy := nodes[0].ID, nodes[3].ID
	if adminCopy == f.admin || createCopy == f.create || reqCopy == f.list {
		t.Fatal("copies must be new rows")
	}
	if got := f.names(t, adminCopy); got != "r:purge" {
		t.Errorf("copied subfolder = %q", got)
	}
	var method, url, body, header string
	if err := db.db.QueryRowContext(ctx,
		`SELECT r.method, r.url, r.body, h.key FROM request r JOIN request_header h ON h.request_id = r.id WHERE r.id = ?`, createCopy).
		Scan(&method, &url, &body, &header); err != nil {
		t.Fatal(err)
	}
	if method != "POST" || url != "{{baseUrl}}/users" || body != "{}" || header != "X-Trace" {
		t.Errorf("copied request = %s %s %s %s", method, url, body, header)
	}
	var vars, headers int
	db.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM folder_var WHERE folder_id = ?`, adminCopy).Scan(&vars)
	db.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM folder_header WHERE folder_id = ? AND mode = 'ignore'`, adminCopy).Scan(&headers)
	if vars != 1 || headers != 1 {
		t.Errorf("copied folder variables %d, headers %d: want 1 and 1", vars, headers)
	}
	// The copy is searchable like any request.
	var n int
	db.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM request_fts WHERE request_fts MATCH 'purge'`).Scan(&n)
	if n != 2 {
		t.Errorf("purge found %d times in the index, want 2", n)
	}

	// Deleting the copy leaves the original whole.
	if err := db.DeleteFolder(ctx, usersCopy); err != nil {
		t.Fatal(err)
	}
	if got := f.names(t, f.users); got != "f:admin r:list r:list (copy) r:create" {
		t.Errorf("original after deleting the copy = %q", got)
	}
}

func TestFolderPaths(t *testing.T) {
	f := newTree(t)
	paths, err := f.db.FolderPaths(ctx, f.ws)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, p := range paths {
		got = append(got, strings.Join(p.Path, " > "))
	}
	want := []string{"orders", "users", "users > admin"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("paths = %v, want %v", got, want)
	}
}

func TestRequestAndFolderRefs(t *testing.T) {
	f := newTree(t)
	r, err := f.db.Request(ctx, f.purge)
	if err != nil || r != (RequestRef{ID: f.purge, FolderID: f.admin, WorkspaceID: f.ws, Name: "purge", Method: "GET"}) {
		t.Fatalf("Request = %+v, %v", r, err)
	}
	fo, err := f.db.Folder(ctx, f.admin)
	if err != nil || fo != (Folder{ID: f.admin, WorkspaceID: f.ws, ParentID: f.users, Name: "admin"}) {
		t.Fatalf("Folder = %+v, %v", fo, err)
	}
	_, err = f.db.Request(ctx, 999)
	wantCode(t, err, apperr.NotFound)
}
