package store

import (
	"reflect"
	"testing"

	"github.com/acoradd/otter/src/internal/apperr"
)

func TestFolderChain(t *testing.T) {
	f := newTree(t)
	if err := f.db.SaveFolderEdit(ctx, f.users, FolderEdit{
		Vars:    []Var{{Key: "scope", Value: "users", Enabled: true}, {Key: "off", Value: "x"}},
		Headers: []Header{{Key: "X-Team", Value: "users", Enabled: true}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := f.db.SaveFolderEdit(ctx, f.admin, FolderEdit{
		Headers: []Header{{Key: "X-Team", Ignore: true, Enabled: true}},
	}); err != nil {
		t.Fatal(err)
	}
	chain, err := f.db.FolderChain(ctx, f.admin)
	if err != nil {
		t.Fatal(err)
	}
	want := []ChainFolder{
		{ID: f.users, Name: "users",
			Vars:    []Var{{Key: "scope", Value: "users", Enabled: true}, {Key: "off", Value: "x"}},
			Headers: []Header{{Key: "X-Team", Value: "users", Enabled: true}}},
		{ID: f.admin, Name: "admin", Vars: []Var{}, Headers: []Header{{Key: "X-Team", Ignore: true, Enabled: true}}},
	}
	if !reflect.DeepEqual(chain, want) {
		t.Fatalf("chain = %+v\nwant  %+v", chain, want)
	}
	if chain, _ := f.db.FolderChain(ctx, 999); len(chain) != 0 {
		t.Errorf("missing folder: %+v", chain)
	}
}

func TestRuntimeVars(t *testing.T) {
	f := newTree(t)
	if _, err := f.db.db.ExecContext(ctx,
		`INSERT INTO runtime_var (workspace_id, environment_id, key, value) VALUES (?, 0, 'token', 'a'), (?, 0, 'id', '1'), (?, 5, 'token', 'b')`,
		f.ws, f.ws, f.ws); err != nil {
		t.Fatal(err)
	}
	got, err := f.db.RuntimeVars(ctx, f.ws, 0)
	want := []Var{{Key: "id", Value: "1", Enabled: true}, {Key: "token", Value: "a", Enabled: true}}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("runtime = %+v, %v", got, err)
	}
	if got, _ := f.db.RuntimeVars(ctx, f.other, 0); len(got) != 0 {
		t.Errorf("another workspace: %+v", got)
	}
	if err := f.db.DeleteRuntimeVar(ctx, f.ws, 0, "token"); err != nil {
		t.Fatal(err)
	}
	if got, _ := f.db.RuntimeVars(ctx, f.ws, 0); len(got) != 1 || got[0].Key != "id" {
		t.Errorf("after delete: %+v", got)
	}
	if got, _ := f.db.RuntimeVars(ctx, f.ws, 5); len(got) != 1 {
		t.Errorf("another environment's runtime variable is kept: %+v", got)
	}
	wantCode(t, f.db.DeleteRuntimeVar(ctx, f.ws, 0, "token"), apperr.NotFound)
}

func TestSetRuntimeVar(t *testing.T) {
	f := newTree(t)
	for _, v := range []string{"a", "b"} {
		if err := f.db.SetRuntimeVar(ctx, f.ws, 0, "token", v); err != nil {
			t.Fatal(err)
		}
	}
	if got, _ := f.db.RuntimeVars(ctx, f.ws, 0); len(got) != 1 || got[0].Value != "b" {
		t.Fatalf("runtime = %+v: set replaces", got)
	}
	wantCode(t, f.db.SetRuntimeVar(ctx, f.ws, 0, "bad key", "x"), CodeInvalidVarKey)
}
