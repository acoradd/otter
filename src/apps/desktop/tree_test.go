package main

import (
	"reflect"
	"testing"

	"github.com/acoradd/otter/src/internal/apperr"
	"github.com/acoradd/otter/src/internal/store"
)

func TestTreeService(t *testing.T) {
	db := openDB(t)
	rec := &emitted{}
	s := NewTreeService(db, rec.emit)
	ws := must(NewWorkspaceService(db, func(string, any) {}).CreateWorkspace(ctx, "API"))
	change := func(folders ...int64) event {
		return event{EventTreeChanged, TreeChange{WorkspaceID: ws, FolderIDs: folders}}
	}
	last := func() event { return rec.events[len(rec.events)-1] }

	users := must(s.CreateFolder(ctx, ws, 0, "users"))
	if !reflect.DeepEqual(last(), change(0)) {
		t.Fatalf("event = %+v", last())
	}
	admin := must(s.CreateFolder(ctx, ws, users, "admin"))
	list := must(s.CreateRequest(ctx, users, "list"))
	if !reflect.DeepEqual(last(), change(users)) {
		t.Fatalf("event = %+v", last())
	}

	got := must(s.GetChildren(ctx, ws, users))
	want := []TreeNode{
		{Kind: NodeFolder, ID: admin, Name: "admin"},
		{Kind: NodeRequest, ID: list, Name: "list", Method: "GET"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("children = %+v, want %+v", got, want)
	}

	if err := s.RenameNode(ctx, NodeRequest, list, "List users"); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(last(), change(users)) {
		t.Fatalf("rename event = %+v", last())
	}

	copyID := must(s.DuplicateNode(ctx, NodeRequest, list, "List users (copy)"))
	if err := s.MoveNode(ctx, NodeRequest, copyID, admin, 0); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(last(), change(users, admin)) {
		t.Fatalf("move event = %+v: both folders changed", last())
	}
	if err := s.MoveNode(ctx, NodeFolder, admin, 0, 0); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(last(), change(users, 0)) {
		t.Fatalf("move to root event = %+v", last())
	}

	choices := must(s.GetFolderChoices(ctx, ws))
	if len(choices) != 2 || !reflect.DeepEqual(choices[0].Path, []string{"admin"}) {
		t.Fatalf("choices = %+v", choices)
	}

	if err := s.DeleteNode(ctx, NodeFolder, admin); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(last(), change(0, admin)) {
		t.Fatalf("delete event = %+v: the parent, and the folder itself for the cache", last())
	}

	n := len(rec.events)
	wantCode(t, s.MoveNode(ctx, NodeFolder, users, users, 0), store.CodeFolderCycle)
	wantCode(t, s.RenameNode(ctx, "bookmark", users, "x"), apperr.InvalidInput)
	wantCode(t, s.DeleteNode(ctx, NodeRequest, 999), apperr.NotFound)
	_, err := s.CreateRequest(ctx, 999, "x")
	wantCode(t, err, apperr.NotFound)
	if len(rec.events) != n {
		t.Fatalf("failed commands emitted events")
	}
}
