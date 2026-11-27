package main

import (
	"context"

	"github.com/acoradd/otter/src/internal/store"
)

// Events of the editors. Their payload is the ID of what changed.
const (
	// EventRequestChanged: a request was saved.
	EventRequestChanged = "request.changed"
	// EventFolderChanged: the variables or headers of a folder were saved.
	EventFolderChanged = "folder.changed"
)

// VarRow is a variable row of a request or a folder.
type VarRow struct {
	Key     string `json:"key"`
	Value   string `json:"value"`
	Enabled bool   `json:"enabled"`
}

// HeaderRow is a header row of a request or a folder. Ignore removes the
// inherited header of that name instead of setting it.
type HeaderRow struct {
	Key     string `json:"key"`
	Value   string `json:"value"`
	Ignore  bool   `json:"ignore"`
	Enabled bool   `json:"enabled"`
}

// BodyFieldRow is a field of a form URL-encoded or multipart body. A file
// field (multipart only) holds a file path in Value.
type BodyFieldRow struct {
	Key         string `json:"key"`
	File        bool   `json:"file"`
	Value       string `json:"value"`
	ContentType string `json:"contentType"`
	Enabled     bool   `json:"enabled"`
}

// RequestDraft is what the request editor saves.
type RequestDraft struct {
	Method string `json:"method"`
	URL    string `json:"url"`
	// BodyType is none, json, xml, text, form-urlencoded, multipart or binary.
	BodyType        string         `json:"bodyType"`
	Body            string         `json:"body"`
	BodyFile        string         `json:"bodyFile"`
	PostScript      string         `json:"postScript"`
	EncodeURL       bool           `json:"encodeUrl"`
	FollowRedirects bool           `json:"followRedirects"`
	MaxRedirects    int            `json:"maxRedirects"`
	TimeoutMS       int            `json:"timeoutMs"`
	Vars            []VarRow       `json:"vars"`
	Headers         []HeaderRow    `json:"headers"`
	BodyFields      []BodyFieldRow `json:"bodyFields"`
}

// RequestView is the request editor: the request and where it sits.
type RequestView struct {
	ID          int64  `json:"id"`
	WorkspaceID int64  `json:"workspaceId"`
	FolderID    int64  `json:"folderId"`
	Name        string `json:"name"`
	// Path is the names of the folders from the root down to the request's.
	Path  []string     `json:"path"`
	Draft RequestDraft `json:"draft"`
}

// FolderDraft is what the folder editor saves.
type FolderDraft struct {
	Vars    []VarRow    `json:"vars"`
	Headers []HeaderRow `json:"headers"`
}

// FolderView is the folder editor.
type FolderView struct {
	ID          int64  `json:"id"`
	WorkspaceID int64  `json:"workspaceId"`
	Name        string `json:"name"`
	// Path is the names of the folders from the root down to this one.
	Path  []string    `json:"path"`
	Draft FolderDraft `json:"draft"`
}

// Dialogs are the native dialogs the editors open.
type Dialogs interface {
	// OpenFile asks for a file, starting in dir; "" when cancelled.
	OpenFile(dir string) (string, error)
}

// EditorService loads and saves requests and folders for their editors.
type EditorService struct {
	db      *store.DB
	emit    func(name string, data any)
	dialogs Dialogs
}

func NewEditorService(db *store.DB, emit func(string, any), dialogs Dialogs) *EditorService {
	return &EditorService{db: db, emit: emit, dialogs: dialogs}
}

// GetRequestView returns a request for its editor.
func (s *EditorService) GetRequestView(ctx context.Context, id int64) (RequestView, error) {
	r, err := s.db.LoadRequest(ctx, id)
	if err != nil {
		return RequestView{}, err
	}
	path, err := s.db.FolderPathOf(ctx, r.Ref.FolderID)
	if err != nil {
		return RequestView{}, err
	}
	e := r.RequestEdit
	d := RequestDraft{
		Method: e.Method, URL: e.URL, BodyType: e.BodyType, Body: e.Body, BodyFile: e.BodyFile,
		PostScript: e.PostScript, EncodeURL: e.EncodeURL, FollowRedirects: e.FollowRedirects,
		MaxRedirects: e.MaxRedirects, TimeoutMS: e.TimeoutMS,
		Vars: varRows(e.Vars), Headers: headerRows(e.Headers), BodyFields: make([]BodyFieldRow, len(e.BodyFields)),
	}
	for i, f := range e.BodyFields {
		d.BodyFields[i] = BodyFieldRow(f)
	}
	return RequestView{ID: id, WorkspaceID: r.Ref.WorkspaceID, FolderID: r.Ref.FolderID, Name: r.Ref.Name, Path: path, Draft: d}, nil
}

// SaveRequest stores what the request editor sends.
func (s *EditorService) SaveRequest(ctx context.Context, id int64, d RequestDraft) error {
	before, err := s.db.Request(ctx, id)
	if err != nil {
		return err
	}
	e := store.RequestEdit{
		Method: d.Method, URL: d.URL, BodyType: d.BodyType, Body: d.Body, BodyFile: d.BodyFile,
		PostScript: d.PostScript, EncodeURL: d.EncodeURL, FollowRedirects: d.FollowRedirects,
		MaxRedirects: d.MaxRedirects, TimeoutMS: d.TimeoutMS,
		Vars: storeVars(d.Vars), Headers: storeHeaders(d.Headers), BodyFields: make([]store.BodyField, len(d.BodyFields)),
	}
	for i, f := range d.BodyFields {
		e.BodyFields[i] = store.BodyField(f)
	}
	if err := s.db.SaveRequest(ctx, id, e); err != nil {
		return err
	}
	s.emit(EventRequestChanged, id)
	if after, err := s.db.Request(ctx, id); err == nil && after.Method != before.Method {
		// The tree shows the method.
		s.emit(EventTreeChanged, TreeChange{WorkspaceID: before.WorkspaceID, FolderIDs: []int64{before.FolderID}})
	}
	return nil
}

// GetFolderView returns a folder for its editor.
func (s *EditorService) GetFolderView(ctx context.Context, id int64) (FolderView, error) {
	f, err := s.db.Folder(ctx, id)
	if err != nil {
		return FolderView{}, err
	}
	e, err := s.db.LoadFolderEdit(ctx, id)
	if err != nil {
		return FolderView{}, err
	}
	path, err := s.db.FolderPathOf(ctx, id)
	if err != nil {
		return FolderView{}, err
	}
	return FolderView{
		ID: id, WorkspaceID: f.WorkspaceID, Name: f.Name, Path: path,
		Draft: FolderDraft{Vars: varRows(e.Vars), Headers: headerRows(e.Headers)},
	}, nil
}

// SaveFolder stores the variables and headers of a folder.
func (s *EditorService) SaveFolder(ctx context.Context, id int64, d FolderDraft) error {
	if err := s.db.SaveFolderEdit(ctx, id, store.FolderEdit{Vars: storeVars(d.Vars), Headers: storeHeaders(d.Headers)}); err != nil {
		return err
	}
	s.emit(EventFolderChanged, id)
	return nil
}

// ChooseFile asks for a file to send (a binary body, a multipart field),
// starting in the folder of current. It returns "" when cancelled.
func (s *EditorService) ChooseFile(current string) (string, error) {
	return s.dialogs.OpenFile(dirOf(current))
}

func varRows(vars []store.Var) []VarRow {
	out := make([]VarRow, len(vars))
	for i, v := range vars {
		out[i] = VarRow(v)
	}
	return out
}

func headerRows(headers []store.Header) []HeaderRow {
	out := make([]HeaderRow, len(headers))
	for i, h := range headers {
		out[i] = HeaderRow(h)
	}
	return out
}

func storeVars(rows []VarRow) []store.Var {
	out := make([]store.Var, len(rows))
	for i, v := range rows {
		out[i] = store.Var(v)
	}
	return out
}

func storeHeaders(rows []HeaderRow) []store.Header {
	out := make([]store.Header, len(rows))
	for i, h := range rows {
		out[i] = store.Header(h)
	}
	return out
}
