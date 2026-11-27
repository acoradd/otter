package main

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/acoradd/otter/src/internal/httpx"
	"github.com/acoradd/otter/src/internal/store"
)

// sendFixture is a workspace "API" with environment dev (baseUrl, a secret
// token), a folder setting Authorization from it, and a request in it.
type sendFixture struct {
	db      *store.DB
	rec     *emitted
	dialogs *fakeDialogs
	s       *SendService
	ws, req int64
	draft   RequestDraft
}

func newSendFixture(t *testing.T, baseURL string) sendFixture {
	t.Helper()
	db := openDB(t)
	quiet := func(string, any) {}
	workspaces := NewWorkspaceService(db, quiet)
	tree := NewTreeService(db, quiet)
	editor := NewEditorService(db, quiet, &fakeDialogs{})
	ws := must(workspaces.CreateWorkspace(ctx, "API"))
	env := must(workspaces.CreateEnvironment(ctx, ws, "dev"))
	if err := workspaces.SaveEnvironmentVars(ctx, env, []EnvVarRow{
		{Key: "baseUrl", Value: baseURL, Enabled: true},
		{Key: "token", Value: "s3cr3t", Secret: true, Enabled: true},
	}); err != nil {
		t.Fatal(err)
	}
	if err := workspaces.SetActiveEnvironment(ctx, ws, env); err != nil {
		t.Fatal(err)
	}
	folder := must(tree.CreateFolder(ctx, ws, 0, "users"))
	if err := editor.SaveFolder(ctx, folder, FolderDraft{Headers: []HeaderRow{{Key: "Authorization", Value: "Bearer {{token}}", Enabled: true}}}); err != nil {
		t.Fatal(err)
	}
	req := must(tree.CreateRequest(ctx, folder, "list"))
	draft := must(editor.GetRequestView(ctx, req)).Draft
	draft.URL = "{{baseUrl}}/users"

	rec := &emitted{}
	dialogs := &fakeDialogs{}
	s := NewSendService(db, NewPreviewService(db, "linux"), rec.emit, fixedClock(), dialogs)
	tr, err := httpx.NewTransport(httpx.Network{})
	if err != nil {
		t.Fatal(err)
	}
	tr.Proxy = nil // the test server is local, whatever the environment says
	s.transport = tr
	return sendFixture{db: db, rec: rec, dialogs: dialogs, s: s, ws: ws, req: req, draft: draft}
}

// fixedClock moves 42 ms at each reading, from a fixed instant.
func fixedClock() func() time.Time {
	t := time.Date(2026, 11, 27, 9, 30, 0, 0, time.UTC)
	return func() time.Time {
		now := t
		t = t.Add(42 * time.Millisecond)
		return now
	}
}

func jsonServer(t *testing.T) (*httptest.Server, *http.Header) {
	t.Helper()
	got := &http.Header{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*got = r.Header.Clone()
		switch r.URL.Path {
		case "/users":
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			http.SetCookie(w, &http.Cookie{Name: "sid", Value: "1"})
			io.WriteString(w, `[{"id":1,"name":"Ada"}]`)
		case "/avatar":
			w.Header().Set("Content-Type", "image/png")
			w.Write([]byte{0x89, 'P', 'N', 'G', 0, 1, 2})
		case "/slow":
			<-r.Context().Done()
		}
	}))
	t.Cleanup(srv.Close)
	return srv, got
}

func TestSendRequest(t *testing.T) {
	srv, got := jsonServer(t)
	f := newSendFixture(t, srv.URL)

	view, err := f.s.SendRequest(ctx, f.req, f.draft)
	if err != nil {
		t.Fatal(err)
	}
	if got.Get("Authorization") != "Bearer s3cr3t" {
		t.Errorf("the secret is sent: %q", got.Get("Authorization"))
	}
	if view.Status != 200 || view.BodyKind != BodyText || view.Body != `[{"id":1,"name":"Ada"}]` ||
		view.ContentType != "application/json" || view.DurationMS != 42 || !view.Saveable || view.ID == 0 {
		t.Fatalf("view = %+v", view)
	}
	for _, h := range view.SentHeaders {
		if h.Key == "Authorization" && h.Value != secretMask {
			t.Errorf("the response view shows the secret: %q", h.Value)
		}
	}
	if !reflect.DeepEqual(f.rec.events, []event{{EventHistoryChanged, f.req}}) {
		t.Errorf("events = %+v", f.rec.events)
	}

	// The history: masked, newest first, with the response.
	list := must(f.s.GetHistory(ctx, f.req))
	if len(list) != 1 || list[0].Status != 200 || list[0].URL != srv.URL+"/users" || list[0].Method != "GET" {
		t.Fatalf("history = %+v", list)
	}
	e := must(f.db.HistoryEntryByID(ctx, view.ID))
	if strings.Contains(e.ReqHeaders, "s3cr3t") || !strings.Contains(e.ReqHeaders, secretMask) {
		t.Errorf("recorded headers = %s", e.ReqHeaders)
	}
	again := must(f.s.GetHistoryResponse(ctx, view.ID))
	if again.Body != view.Body || again.ContentType != "application/json" || again.Status != 200 {
		t.Errorf("from the history = %+v", again)
	}

	// The cookie the server set comes back, until cleared.
	f.s.SendRequest(ctx, f.req, f.draft)
	if got.Get("Cookie") != "sid=1" {
		t.Errorf("Cookie = %q", got.Get("Cookie"))
	}
	f.s.ClearCookies(f.ws)
	f.s.SendRequest(ctx, f.req, f.draft)
	if got.Get("Cookie") != "" {
		t.Errorf("after clearing, Cookie = %q", got.Get("Cookie"))
	}
}

func TestSendUnsavedDraft(t *testing.T) {
	srv, got := jsonServer(t)
	f := newSendFixture(t, srv.URL)
	f.draft.Headers = []HeaderRow{{Key: "X-Draft", Value: "yes", Enabled: true}} // not saved
	if _, err := f.s.SendRequest(ctx, f.req, f.draft); err != nil {
		t.Fatal(err)
	}
	if got.Get("X-Draft") != "yes" {
		t.Error("what the editor holds is what is sent")
	}
}

func TestSendBinaryAndSave(t *testing.T) {
	srv, _ := jsonServer(t)
	f := newSendFixture(t, srv.URL)
	f.draft.URL = "{{baseUrl}}/avatar"
	view := must(f.s.SendRequest(ctx, f.req, f.draft))
	if view.BodyKind != BodyBinary || view.Body != "" || view.Size != 7 {
		t.Fatalf("binary view = %+v", view)
	}

	dest := filepath.Join(t.TempDir(), "out.png")
	f.dialogs.save = dest
	if got := must(f.s.SaveResponseBody(ctx, view.ID)); got != dest {
		t.Fatalf("saved to %q", got)
	}
	data, _ := os.ReadFile(dest)
	if len(data) != 7 || data[1] != 'P' {
		t.Errorf("saved %v", data)
	}
	if f.dialogs.names[0] != "response.png" {
		t.Errorf("suggested name %q", f.dialogs.names[0])
	}

	f.dialogs.save = "" // cancelled
	if got := must(f.s.SaveResponseBody(ctx, view.ID)); got != "" {
		t.Errorf("cancelled save returned %q", got)
	}
	// Not in memory any more, not in the history (binary): gone.
	f.s.bodies = map[int64][]byte{}
	wantCode(t, second(f.s.SaveResponseBody(ctx, view.ID)), CodeBodyGone)
}

func second(_ string, err error) error { return err }

func TestSendFailureIsRecorded(t *testing.T) {
	srv, _ := jsonServer(t)
	url := srv.URL
	srv.Close()
	f := newSendFixture(t, url)
	_, err := f.s.SendRequest(ctx, f.req, f.draft)
	wantCode(t, err, httpx.CodeConnRefused)
	list := must(f.s.GetHistory(ctx, f.req))
	if len(list) != 1 || list[0].Error != string(httpx.CodeConnRefused) || list[0].Status != 0 {
		t.Fatalf("history = %+v", list)
	}
	e := must(f.db.HistoryEntryByID(ctx, list[0].ID))
	if strings.Contains(e.ReqHeaders, "s3cr3t") {
		t.Errorf("a failed send masks secrets too: %s", e.ReqHeaders)
	}
	view := must(f.s.GetHistoryResponse(ctx, list[0].ID))
	if view.Error != string(httpx.CodeConnRefused) || view.BodyKind != BodyEmpty {
		t.Errorf("failed view = %+v", view)
	}
}

func TestCancelRequest(t *testing.T) {
	srv, _ := jsonServer(t)
	f := newSendFixture(t, srv.URL)
	f.draft.URL = "{{baseUrl}}/slow"
	done := make(chan error, 1)
	go func() {
		_, err := f.s.SendRequest(ctx, f.req, f.draft)
		done <- err
	}()
	deadline := time.After(5 * time.Second)
	for {
		f.s.mu.Lock()
		running := f.s.inflight[f.req] != nil
		f.s.mu.Unlock()
		if running {
			break
		}
		select {
		case <-deadline:
			t.Fatal("the send never started")
		case <-time.After(5 * time.Millisecond):
		}
	}
	f.s.CancelRequest(f.req)
	select {
	case err := <-done:
		wantCode(t, err, httpx.CodeCancelled)
	case <-time.After(5 * time.Second):
		t.Fatal("cancel did not stop the send")
	}
	f.s.CancelRequest(f.req) // nothing running: no effect
}

func TestSendRequestErrors(t *testing.T) {
	f := newSendFixture(t, "http://127.0.0.1:1")
	_, err := f.s.SendRequest(context.Background(), 999, f.draft)
	wantCode(t, err, "NOT_FOUND")
	_, err = f.s.GetHistoryResponse(ctx, 999)
	wantCode(t, err, "NOT_FOUND")
}

func TestSuggestedName(t *testing.T) {
	tests := map[[2]string]string{
		{"https://h/files/report.pdf", "application/pdf"}: "report.pdf",
		{"https://h/users", "application/json"}:           "response.json",
		{"https://h/", "text/html"}:                       "response.html",
		{"https://h/x", "application/octet-stream"}:       "response.bin",
	}
	for in, want := range tests {
		if got := suggestedName(in[0], in[1]); got != want && !(want == "response.bin" && strings.HasPrefix(got, "response.")) {
			t.Errorf("%v: %q, want %q", in, got, want)
		}
	}
}

func TestIsText(t *testing.T) {
	tests := []struct {
		contentType string
		body        string
		want        bool
	}{
		{"application/json", `{"a":1}`, true},
		{"text/html", "<p>é</p>", true},
		{"", "<html></html>", true},
		{"application/octet-stream", `{"mislabelled":true}`, true},
		{"application/octet-stream", "\x89PNG\x00\x01", false},
		{"image/png", "looks like text", false},
		{"text/plain", "\xff\xfe", false},
	}
	for _, tt := range tests {
		if got := isText(tt.contentType, []byte(tt.body)); got != tt.want {
			t.Errorf("isText(%q, %q) = %v, want %v", tt.contentType, tt.body, got, tt.want)
		}
	}
}
