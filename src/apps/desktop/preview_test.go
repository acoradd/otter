package main

import (
	"os"
	"testing"

	"github.com/acoradd/otter/src/internal/apperr"
	"github.com/acoradd/otter/src/internal/resolve"
)

func TestPreviewLoadsEveryLayer(t *testing.T) {
	db := openDB(t)
	workspaces := NewWorkspaceService(db, func(string, any) {})
	tree := NewTreeService(db, func(string, any) {})
	editor := NewEditorService(db, func(string, any) {}, &fakeDialogs{})
	s := NewPreviewService(db, "windows")

	ws := must(workspaces.CreateWorkspace(ctx, "API"))
	api := must(tree.CreateFolder(ctx, ws, 0, "api"))
	users := must(tree.CreateFolder(ctx, ws, api, "users"))
	req := must(tree.CreateRequest(ctx, users, "list"))
	env := must(workspaces.CreateEnvironment(ctx, ws, "dev"))
	if err := workspaces.SaveEnvironmentVars(ctx, env, []EnvVarRow{
		{Key: "baseUrl", Value: "https://dev.test", Enabled: true},
		{Key: "token", Value: "s3cr3t", Secret: true, Enabled: true},
	}); err != nil {
		t.Fatal(err)
	}
	if err := workspaces.SetActiveEnvironment(ctx, ws, env); err != nil {
		t.Fatal(err)
	}
	if err := editor.SaveFolder(ctx, api, FolderDraft{
		Vars:    []VarRow{{Key: "version", Value: "v1", Enabled: true}},
		Headers: []HeaderRow{{Key: "Authorization", Value: "Bearer {{token}}", Enabled: true}},
	}); err != nil {
		t.Fatal(err)
	}

	// The draft, not what is saved: the preview follows typing.
	draft := must(editor.GetRequestView(ctx, req)).Draft
	draft.URL = "{{baseUrl}}/{{version}}/users/{{id}}"
	p := must(s.GetRequestPreview(ctx, req, draft))

	if p.URL != "https://dev.test/v1/users/{{id}}" {
		t.Errorf("URL = %q", p.URL)
	}
	if len(p.Missing) != 1 || p.Missing[0] != "id" {
		t.Errorf("missing = %v", p.Missing)
	}
	var auth *resolve.ResolvedHeader
	for i := range p.Headers {
		if p.Headers[i].Key == "Authorization" {
			auth = &p.Headers[i]
		}
	}
	if auth == nil || auth.Value != "Bearer s3cr3t" || !auth.Secret || auth.Source.Kind != resolve.SourceFolder ||
		auth.Source.Depth != 1 || len(auth.Source.Path) != 1 || auth.Source.Path[0] != "api" {
		t.Errorf("Authorization = %+v", auth)
	}
	for _, h := range p.Headers {
		if h.Key == "User-Agent" && h.Value != resolve.UserAgent("windows") {
			t.Errorf("User-Agent for the service's OS: %q", h.Value)
		}
	}

	// The folder level, with the folder editor's draft.
	fp := must(s.GetFolderPreview(ctx, users, FolderDraft{Vars: []VarRow{{Key: "version", Value: "v2", Enabled: true}}}))
	for _, v := range fp.Vars {
		if v.Key == "version" && (v.Value != "v2" || v.Source.Depth != 0 || len(v.Shadowed) != 1) {
			t.Errorf("version at users = %+v", v)
		}
	}

	_, err := s.GetRequestPreview(ctx, 999, draft)
	wantCode(t, err, apperr.NotFound)
	_, err = s.GetFolderPreview(ctx, 999, FolderDraft{})
	wantCode(t, err, apperr.NotFound)
}

func TestPreviewWarnsAboutMissingFiles(t *testing.T) {
	db := openDB(t)
	ws := must(NewWorkspaceService(db, func(string, any) {}).CreateWorkspace(ctx, "API"))
	tree := NewTreeService(db, func(string, any) {})
	req := must(tree.CreateRequest(ctx, must(tree.CreateFolder(ctx, ws, 0, "f")), "upload"))
	there := t.TempDir() + "/there.bin"
	if err := os.WriteFile(there, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	d := RequestDraft{Method: "POST", URL: "https://h", BodyType: "multipart", TimeoutMS: 1000, BodyFields: []BodyFieldRow{
		{Key: "a", File: true, Value: there, Enabled: true},
		{Key: "b", File: true, Value: "/nowhere/gone.bin", Enabled: true},
	}}
	p := must(NewPreviewService(db, "linux").GetRequestPreview(ctx, req, d))
	if len(p.Warnings) != 1 || p.Warnings[0].Code != WarnFileNotFound || p.Warnings[0].Params["path"] != "/nowhere/gone.bin" {
		t.Fatalf("warnings = %+v", p.Warnings)
	}
}
