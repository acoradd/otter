package main

import (
	"context"
	"os"
	"strings"

	"github.com/acoradd/otter/src/internal/resolve"
	"github.com/acoradd/otter/src/internal/store"
)

// PreviewService resolves what a request (or a folder) will use: the same
// code path builds what Send sends.
type PreviewService struct {
	db *store.DB
	// os picks the automatic User-Agent (runtime.GOOS).
	os string
}

func NewPreviewService(db *store.DB, os string) *PreviewService {
	return &PreviewService{db: db, os: os}
}

// GetRequestPreview resolves a request as the editor holds it, saved or not.
func (s *PreviewService) GetRequestPreview(ctx context.Context, requestID int64, d RequestDraft) (resolve.Preview, error) {
	in, err := s.requestInput(ctx, requestID, d)
	if err != nil {
		return resolve.Preview{}, err
	}
	p := resolve.Resolve(in)
	checkFiles(&p)
	return p, nil
}

// WarnFileNotFound is a file to send that cannot be read now (param path).
// resolve is pure and cannot know: the service looks.
const WarnFileNotFound = "FILE_NOT_FOUND"

// checkFiles warns about the files of the body that are not there; the
// request can still be sent, without them.
func checkFiles(p *resolve.Preview) {
	var files []string
	switch p.BodyType {
	case "binary":
		if strings.TrimSpace(p.BodyFile) != "" {
			files = append(files, p.BodyFile)
		}
	case "multipart":
		for _, f := range p.BodyFields {
			if f.File {
				files = append(files, f.Value)
			}
		}
	}
	for _, f := range files {
		if info, err := os.Stat(f); err != nil || info.IsDir() {
			p.Warnings = append(p.Warnings, resolve.Warning{Code: WarnFileNotFound, Params: map[string]string{"path": f}})
		}
	}
}

// GetFolderPreview resolves the variables and headers at the level of a
// folder, with its rows as the editor holds them.
func (s *PreviewService) GetFolderPreview(ctx context.Context, folderID int64, d FolderDraft) (resolve.Preview, error) {
	f, err := s.db.Folder(ctx, folderID)
	if err != nil {
		return resolve.Preview{}, err
	}
	in, err := s.input(ctx, f.WorkspaceID, folderID)
	if err != nil {
		return resolve.Preview{}, err
	}
	last := &in.Folders[len(in.Folders)-1]
	last.Vars, last.Headers = resolveVars(storeVars(d.Vars)), resolveHeaders(storeHeaders(d.Headers))
	return resolve.Resolve(in), nil
}

// requestInput loads every layer of a request, with the editor's draft as
// the request layer. Send resolves from it too.
func (s *PreviewService) requestInput(ctx context.Context, requestID int64, d RequestDraft) (resolve.Input, error) {
	r, err := s.db.Request(ctx, requestID)
	if err != nil {
		return resolve.Input{}, err
	}
	in, err := s.input(ctx, r.WorkspaceID, r.FolderID)
	if err != nil {
		return resolve.Input{}, err
	}
	fields := make([]resolve.Field, len(d.BodyFields))
	for i, f := range d.BodyFields {
		fields[i] = resolve.Field{Key: f.Key, Value: f.Value, File: f.File, ContentType: f.ContentType, Enabled: f.Enabled}
	}
	in.Request = &resolve.Request{
		Method: d.Method, URL: d.URL, BodyType: d.BodyType, Body: d.Body, BodyFile: d.BodyFile, Fields: fields,
		Vars: resolveVars(storeVars(d.Vars)), Headers: resolveHeaders(storeHeaders(d.Headers)),
	}
	return in, nil
}

// input loads the folder chain, the active environment and the runtime
// variables of a workspace.
func (s *PreviewService) input(ctx context.Context, workspaceID, folderID int64) (resolve.Input, error) {
	in := resolve.Input{OS: s.os, Runtime: []resolve.Var{}}
	chain, err := s.db.FolderChain(ctx, folderID)
	if err != nil {
		return in, err
	}
	var path []string
	for _, f := range chain {
		path = append(append([]string{}, path...), f.Name)
		in.Folders = append(in.Folders, resolve.Folder{
			ID: f.ID, Name: f.Name, Path: path, Vars: resolveVars(f.Vars), Headers: resolveHeaders(f.Headers),
		})
	}
	ws, err := s.db.Workspace(ctx, workspaceID)
	if err != nil {
		return in, err
	}
	if ws.ActiveEnvironmentID != 0 {
		env, err := s.db.Environment(ctx, ws.ActiveEnvironmentID)
		if err != nil {
			return in, err
		}
		vars, err := s.db.EnvVars(ctx, env.ID)
		if err != nil {
			return in, err
		}
		e := &resolve.Env{ID: env.ID, Name: env.Name}
		for _, v := range vars {
			e.Vars = append(e.Vars, resolve.Var{Key: v.Key, Value: v.Value, Enabled: v.Enabled, Secret: v.Secret})
		}
		in.Env = e
	}
	runtime, err := s.db.RuntimeVars(ctx, workspaceID, ws.ActiveEnvironmentID)
	if err != nil {
		return in, err
	}
	in.Runtime = resolveVars(runtime)
	return in, nil
}

func resolveVars(vars []store.Var) []resolve.Var {
	out := make([]resolve.Var, len(vars))
	for i, v := range vars {
		out[i] = resolve.Var{Key: v.Key, Value: v.Value, Enabled: v.Enabled}
	}
	return out
}

func resolveHeaders(headers []store.Header) []resolve.Header {
	out := make([]resolve.Header, len(headers))
	for i, h := range headers {
		out[i] = resolve.Header{Key: h.Key, Value: h.Value, Ignore: h.Ignore, Enabled: h.Enabled}
	}
	return out
}
