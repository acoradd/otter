package main

import (
	"reflect"
	"testing"

	"github.com/acoradd/otter/src/internal/apperr"
	"github.com/acoradd/otter/src/internal/store"
)

// fakeDialogs answers the file dialogs with fixed paths and records where they opened.
type fakeDialogs struct {
	answer string
	save   string
	dirs   []string
	names  []string
}

func (f *fakeDialogs) SaveFile(dir, name string) (string, error) {
	f.dirs = append(f.dirs, dir)
	f.names = append(f.names, name)
	return f.save, nil
}

func (f *fakeDialogs) OpenFile(dir string) (string, error) {
	f.dirs = append(f.dirs, dir)
	return f.answer, nil
}

func sampleDraft() RequestDraft {
	return RequestDraft{
		Method: "post", URL: "{{baseUrl}}/customers", BodyType: "json", Body: `{"name":"Ada"}`,
		EncodeURL: true, FollowRedirects: true, MaxRedirects: 10, TimeoutMS: 30000,
		Vars:       []VarRow{{Key: "tenant", Value: "acme", Enabled: true}},
		Headers:    []HeaderRow{{Key: "Idempotency-Key", Value: "{{$uuid}}", Enabled: true}, {Key: "Accept", Ignore: true, Enabled: true}},
		BodyFields: []BodyFieldRow{},
	}
}

func TestEditorRequest(t *testing.T) {
	db := openDB(t)
	rec := &emitted{}
	ws := must(NewWorkspaceService(db, func(string, any) {}).CreateWorkspace(ctx, "API"))
	tree := NewTreeService(db, func(string, any) {})
	customers := must(tree.CreateFolder(ctx, ws, 0, "customers"))
	cards := must(tree.CreateFolder(ctx, ws, customers, "cards"))
	id := must(tree.CreateRequest(ctx, cards, "Add card"))
	s := NewEditorService(db, rec.emit, &fakeDialogs{})

	view := must(s.GetRequestView(ctx, id))
	if view.Name != "Add card" || view.FolderID != cards || view.WorkspaceID != ws ||
		!reflect.DeepEqual(view.Path, []string{"customers", "cards"}) || view.Draft.Method != "GET" {
		t.Fatalf("new request view = %+v", view)
	}

	if err := s.SaveRequest(ctx, id, sampleDraft()); err != nil {
		t.Fatal(err)
	}
	wantEvents := []event{
		{EventRequestChanged, id},
		{EventTreeChanged, TreeChange{WorkspaceID: ws, FolderIDs: []int64{cards}}}, // the method changed
	}
	if !reflect.DeepEqual(rec.events, wantEvents) {
		t.Fatalf("events = %+v, want %+v", rec.events, wantEvents)
	}
	want := sampleDraft()
	want.Method = "POST"
	want.Headers[1].Value = ""
	if got := must(s.GetRequestView(ctx, id)).Draft; !reflect.DeepEqual(got, want) {
		t.Fatalf("draft = %+v\nwant  %+v", got, want)
	}

	// Saving again without changing the method leaves the tree alone.
	rec.events = nil
	if err := s.SaveRequest(ctx, id, want); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(rec.names(), []string{EventRequestChanged}) {
		t.Fatalf("events = %v", rec.names())
	}

	rec.events = nil
	bad := sampleDraft()
	bad.Method = "NOT A METHOD"
	wantCode(t, s.SaveRequest(ctx, id, bad), store.CodeInvalidMethod)
	wantCode(t, s.SaveRequest(ctx, 999, sampleDraft()), apperr.NotFound)
	if len(rec.events) != 0 {
		t.Fatalf("failed saves emitted %v", rec.names())
	}
}

func TestEditorFolder(t *testing.T) {
	db := openDB(t)
	rec := &emitted{}
	ws := must(NewWorkspaceService(db, func(string, any) {}).CreateWorkspace(ctx, "API"))
	folder := must(NewTreeService(db, func(string, any) {}).CreateFolder(ctx, ws, 0, "auth"))
	s := NewEditorService(db, rec.emit, &fakeDialogs{})

	d := FolderDraft{
		Vars:    []VarRow{{Key: "realm", Value: "api", Enabled: true}},
		Headers: []HeaderRow{{Key: "Authorization", Value: "Bearer {{token}}", Enabled: true}},
	}
	if err := s.SaveFolder(ctx, folder, d); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(rec.events, []event{{EventFolderChanged, folder}}) {
		t.Fatalf("events = %+v", rec.events)
	}
	view := must(s.GetFolderView(ctx, folder))
	want := FolderView{ID: folder, WorkspaceID: ws, Name: "auth", Path: []string{"auth"}, Draft: d}
	if !reflect.DeepEqual(view, want) {
		t.Fatalf("view = %+v, want %+v", view, want)
	}
	_, err := s.GetFolderView(ctx, 999)
	wantCode(t, err, apperr.NotFound)
}

func TestChooseFileStartsNextToTheCurrentOne(t *testing.T) {
	dialogs := &fakeDialogs{answer: "/home/me/avatar.png"}
	s := NewEditorService(openDB(t), func(string, any) {}, dialogs)
	if got := must(s.ChooseFile("/home/me/photos/old.png")); got != "/home/me/avatar.png" {
		t.Fatalf("ChooseFile = %q", got)
	}
	must(s.ChooseFile(""))
	if !reflect.DeepEqual(dialogs.dirs, []string{"/home/me/photos", ""}) {
		t.Fatalf("dialog opened in %v", dialogs.dirs)
	}
}
