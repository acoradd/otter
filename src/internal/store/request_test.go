package store

import (
	"reflect"
	"testing"

	"github.com/acoradd/otter/src/internal/apperr"
)

func validEdit() RequestEdit {
	return RequestEdit{
		Method:          "post",
		URL:             "{{baseUrl}}/users?page=1",
		BodyType:        BodyMultipart,
		Body:            `{"name":"Ada"}`,
		BodyFile:        " /tmp/avatar.png ",
		PostScript:      `bru.setVar("id", res.body.id)`,
		EncodeURL:       false,
		FollowRedirects: true,
		MaxRedirects:    5,
		TimeoutMS:       10000,
		Vars:            []Var{{Key: " page ", Value: "1", Enabled: true}, {}},
		Headers: []Header{
			{Key: "X-Trace", Value: "{{traceId}}", Enabled: true},
			{Key: "Authorization", Value: "kept?", Ignore: true, Enabled: true},
			{Key: "  ", Value: ""},
		},
		BodyFields: []BodyField{
			{Key: "name", Value: "Ada", Enabled: true},
			{Key: "avatar", File: true, Value: "/tmp/avatar.png", ContentType: " image/png ", Enabled: true},
			{},
		},
	}
}

func TestLoadNewRequestDefaults(t *testing.T) {
	f := newTree(t)
	r, err := f.db.LoadRequest(ctx, f.list)
	if err != nil {
		t.Fatal(err)
	}
	want := Request{
		Ref: RequestRef{ID: f.list, FolderID: f.users, WorkspaceID: f.ws, Name: "list", Method: "GET"},
		RequestEdit: RequestEdit{
			Method: "GET", BodyType: BodyNone, EncodeURL: true, FollowRedirects: true, MaxRedirects: 10, TimeoutMS: 30000,
			Vars: []Var{}, Headers: []Header{}, BodyFields: []BodyField{},
		},
	}
	if !reflect.DeepEqual(r, want) {
		t.Fatalf("new request = %+v\nwant %+v", r, want)
	}
}

func TestSaveRequestRoundTrip(t *testing.T) {
	f := newTree(t)
	if err := f.db.SaveRequest(ctx, f.list, validEdit()); err != nil {
		t.Fatal(err)
	}
	r, err := f.db.LoadRequest(ctx, f.list)
	if err != nil {
		t.Fatal(err)
	}
	want := RequestEdit{
		Method:          "POST",
		URL:             "{{baseUrl}}/users?page=1",
		BodyType:        BodyMultipart,
		Body:            `{"name":"Ada"}`,
		BodyFile:        "/tmp/avatar.png",
		PostScript:      `bru.setVar("id", res.body.id)`,
		FollowRedirects: true,
		MaxRedirects:    5,
		TimeoutMS:       10000,
		Vars:            []Var{{Key: "page", Value: "1", Enabled: true}},
		Headers: []Header{
			{Key: "X-Trace", Value: "{{traceId}}", Enabled: true},
			{Key: "Authorization", Ignore: true, Enabled: true},
		},
		BodyFields: []BodyField{
			{Key: "name", Value: "Ada", Enabled: true},
			{Key: "avatar", File: true, Value: "/tmp/avatar.png", ContentType: "image/png", Enabled: true},
		},
	}
	if !reflect.DeepEqual(r.RequestEdit, want) {
		t.Fatalf("saved = %+v\nwant  %+v", r.RequestEdit, want)
	}
	if r.Ref.Method != "POST" {
		t.Errorf("the tree's method = %q, want POST", r.Ref.Method)
	}

	// Saving again replaces the lists.
	e := validEdit()
	e.Vars, e.Headers, e.BodyFields = nil, nil, nil
	if err := f.db.SaveRequest(ctx, f.list, e); err != nil {
		t.Fatal(err)
	}
	r, _ = f.db.LoadRequest(ctx, f.list)
	if len(r.Vars)+len(r.Headers)+len(r.BodyFields) != 0 {
		t.Fatalf("lists not cleared: %+v", r.RequestEdit)
	}
}

func TestSaveRequestRefuses(t *testing.T) {
	tests := []struct {
		name   string
		change func(e *RequestEdit)
		want   apperr.Code
	}{
		{"an empty method", func(e *RequestEdit) { e.Method = " " }, CodeInvalidMethod},
		{"a method with a space", func(e *RequestEdit) { e.Method = "GET X" }, CodeInvalidMethod},
		{"an unknown body type", func(e *RequestEdit) { e.BodyType = "graphql" }, apperr.InvalidInput},
		{"a zero timeout", func(e *RequestEdit) { e.TimeoutMS = 0 }, CodeInvalidTimeout},
		{"a timeout over an hour", func(e *RequestEdit) { e.TimeoutMS = 3_600_001 }, CodeInvalidTimeout},
		{"negative redirects", func(e *RequestEdit) { e.MaxRedirects = -1 }, CodeInvalidMaxRedirects},
		{"too many redirects", func(e *RequestEdit) { e.MaxRedirects = 101 }, CodeInvalidMaxRedirects},
		{"a bad variable name", func(e *RequestEdit) { e.Vars = []Var{{Key: "a b", Value: "1"}} }, CodeInvalidVarKey},
		{"a header value without a name", func(e *RequestEdit) { e.Headers = []Header{{Value: "x"}} }, CodeHeaderNameRequired},
		{"a field value without a name", func(e *RequestEdit) { e.BodyFields = []BodyField{{Value: "x"}} }, CodeFieldNameRequired},
		{"a file field outside multipart", func(e *RequestEdit) { e.BodyType = BodyForm }, apperr.InvalidInput},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newTree(t)
			e := validEdit()
			tt.change(&e)
			wantCode(t, f.db.SaveRequest(ctx, f.list, e), tt.want)
			if r, _ := f.db.LoadRequest(ctx, f.list); r.Ref.Method != "GET" || len(r.Headers) != 0 {
				t.Errorf("a refused save changed the request: %+v", r.RequestEdit)
			}
		})
	}
	f := newTree(t)
	wantCode(t, f.db.SaveRequest(ctx, 999, validEdit()), apperr.NotFound)
	_, err := f.db.LoadRequest(ctx, 999)
	wantCode(t, err, apperr.NotFound)
}

func TestCustomMethodsAreKept(t *testing.T) {
	f := newTree(t)
	for _, m := range []string{"purge", "PROPFIND", "M-SEARCH"} {
		e := validEdit()
		e.Method = m
		if err := f.db.SaveRequest(ctx, f.list, e); err != nil {
			t.Fatalf("%s: %v", m, err)
		}
	}
}

func TestFolderEdit(t *testing.T) {
	f := newTree(t)
	e, err := f.db.LoadFolderEdit(ctx, f.users)
	if err != nil || len(e.Vars) != 0 || len(e.Headers) != 0 {
		t.Fatalf("new folder = %+v, %v", e, err)
	}
	in := FolderEdit{
		Vars: []Var{{Key: "scope", Value: "users", Enabled: true}},
		Headers: []Header{
			{Key: "Authorization", Value: "Bearer {{token}}", Enabled: true},
			{Key: "User-Agent", Ignore: true, Enabled: true},
		},
	}
	if err := f.db.SaveFolderEdit(ctx, f.users, in); err != nil {
		t.Fatal(err)
	}
	got, _ := f.db.LoadFolderEdit(ctx, f.users)
	if !reflect.DeepEqual(got, in) {
		t.Fatalf("folder = %+v, want %+v", got, in)
	}

	wantCode(t, f.db.SaveFolderEdit(ctx, 999, in), apperr.NotFound)
	wantCode(t, f.db.SaveFolderEdit(ctx, f.users, FolderEdit{Headers: []Header{{Value: "x"}}}), CodeHeaderNameRequired)
	_, err = f.db.LoadFolderEdit(ctx, 999)
	wantCode(t, err, apperr.NotFound)

	// Deleting the folder deletes its variables and headers.
	if err := f.db.DeleteFolder(ctx, f.users); err != nil {
		t.Fatal(err)
	}
	var n int
	f.db.db.QueryRowContext(ctx, `SELECT (SELECT COUNT(*) FROM folder_var) + (SELECT COUNT(*) FROM folder_header)`).Scan(&n)
	if n != 0 {
		t.Errorf("%d rows left after deleting the folder", n)
	}
}

func TestSavedRequestIsSearchable(t *testing.T) {
	f := newTree(t)
	e := validEdit()
	e.URL = "https://api.test/v3/invoices"
	if err := f.db.SaveRequest(ctx, f.list, e); err != nil {
		t.Fatal(err)
	}
	var n int
	f.db.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM request_fts_tri WHERE request_fts_tri MATCH '"invoices"'`).Scan(&n)
	if n != 1 {
		t.Errorf("new URL found %d times, want 1", n)
	}
}
