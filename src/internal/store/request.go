package store

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"strings"

	"github.com/acoradd/otter/src/internal/apperr"
)

const (
	// CodeInvalidMethod is an HTTP method that is not a token (GET, PURGE…).
	CodeInvalidMethod apperr.Code = "INVALID_METHOD"
	// CodeInvalidTimeout is a timeout outside 1 ms to 1 hour.
	CodeInvalidTimeout apperr.Code = "INVALID_TIMEOUT"
	// CodeInvalidMaxRedirects is a redirect limit outside 0 to 100.
	CodeInvalidMaxRedirects apperr.Code = "INVALID_MAX_REDIRECTS"
	// CodeHeaderNameRequired is a header with a value but no name.
	CodeHeaderNameRequired apperr.Code = "HEADER_NAME_REQUIRED"
	// CodeFieldNameRequired is a body field with a value but no name.
	CodeFieldNameRequired apperr.Code = "FIELD_NAME_REQUIRED"
)

// Body types (request.body_type).
const (
	BodyNone       = "none"
	BodyJSON       = "json"
	BodyXML        = "xml"
	BodyText       = "text"
	BodyForm       = "form-urlencoded"
	BodyMultipart  = "multipart"
	BodyBinaryFile = "binary"
)

var bodyTypes = map[string]bool{
	BodyNone: true, BodyJSON: true, BodyXML: true, BodyText: true,
	BodyForm: true, BodyMultipart: true, BodyBinaryFile: true,
}

const (
	maxTimeoutMS    = 3_600_000
	maxMaxRedirects = 100
)

var methodToken = regexp.MustCompile(`^[A-Z][A-Z0-9_-]*$`)

// Var is a variable of a request or a folder.
type Var struct {
	Key     string
	Value   string
	Enabled bool
}

// Header is a header row of a request or a folder. Ignore removes the
// inherited header of that name instead of setting one (mode 'ignore').
type Header struct {
	Key     string
	Value   string
	Ignore  bool
	Enabled bool
}

// BodyField is a field of a form URL-encoded or multipart body. A file
// field (multipart only) holds a file path in Value.
type BodyField struct {
	Key         string
	File        bool
	Value       string
	ContentType string
	Enabled     bool
}

// RequestEdit is what the request editor saves: everything but the name and
// the place in the tree.
type RequestEdit struct {
	Method          string
	URL             string
	BodyType        string
	Body            string
	BodyFile        string
	PostScript      string
	EncodeURL       bool
	FollowRedirects bool
	MaxRedirects    int
	TimeoutMS       int
	Vars            []Var
	Headers         []Header
	BodyFields      []BodyField
}

// Request is a request with everything in it.
type Request struct {
	// Ref is where the request sits in the tree.
	Ref RequestRef
	RequestEdit
}

// LoadRequest returns a request with its variables, headers and body fields.
func (d *DB) LoadRequest(ctx context.Context, id int64) (Request, error) {
	ref, err := requestRow(ctx, d.db, id)
	if err != nil {
		return Request{}, err
	}
	r := Request{Ref: ref}
	e := &r.RequestEdit
	err = d.db.QueryRowContext(ctx,
		`SELECT method, url, body_type, body, body_file, post_script, encode_url, follow_redirects, max_redirects, timeout_ms
		 FROM request WHERE id = ?`, id).
		Scan(&e.Method, &e.URL, &e.BodyType, &e.Body, &e.BodyFile, &e.PostScript, &e.EncodeURL, &e.FollowRedirects, &e.MaxRedirects, &e.TimeoutMS)
	if errors.Is(err, sql.ErrNoRows) {
		return Request{}, apperr.New(apperr.NotFound)
	}
	if err != nil {
		return Request{}, err
	}
	if e.Vars, err = loadVars(ctx, d.db, `SELECT key, value, enabled FROM request_var WHERE request_id = ? ORDER BY seq, id`, id); err != nil {
		return Request{}, err
	}
	if e.Headers, err = loadHeaders(ctx, d.db, `SELECT key, value, mode, enabled FROM request_header WHERE request_id = ? ORDER BY seq, id`, id); err != nil {
		return Request{}, err
	}
	rows, err := d.db.QueryContext(ctx,
		`SELECT key, kind, value, content_type, enabled FROM request_body_field WHERE request_id = ? ORDER BY seq, id`, id)
	if err != nil {
		return Request{}, err
	}
	defer rows.Close()
	e.BodyFields = []BodyField{}
	for rows.Next() {
		var f BodyField
		var kind string
		if err := rows.Scan(&f.Key, &kind, &f.Value, &f.ContentType, &f.Enabled); err != nil {
			return Request{}, err
		}
		f.File = kind == "file"
		e.BodyFields = append(e.BodyFields, f)
	}
	return r, rows.Err()
}

// SaveRequest checks and stores what the editor sends, in one transaction.
// The method is upper-cased; blank rows are dropped.
func (d *DB) SaveRequest(ctx context.Context, id int64, e RequestEdit) error {
	e.Method = strings.ToUpper(strings.TrimSpace(e.Method))
	if !methodToken.MatchString(e.Method) {
		return apperr.New(CodeInvalidMethod).With("method", e.Method)
	}
	if !bodyTypes[e.BodyType] {
		return apperr.New(apperr.InvalidInput)
	}
	if e.TimeoutMS < 1 || e.TimeoutMS > maxTimeoutMS {
		return apperr.New(CodeInvalidTimeout)
	}
	if e.MaxRedirects < 0 || e.MaxRedirects > maxMaxRedirects {
		return apperr.New(CodeInvalidMaxRedirects)
	}
	vars, err := cleanVars(e.Vars)
	if err != nil {
		return err
	}
	headers, err := cleanHeaders(e.Headers)
	if err != nil {
		return err
	}
	fields := make([]BodyField, 0, len(e.BodyFields))
	for _, f := range e.BodyFields {
		f.Key = strings.TrimSpace(f.Key)
		f.ContentType = strings.TrimSpace(f.ContentType)
		if f.Key == "" && f.Value == "" {
			continue
		}
		if f.Key == "" {
			return apperr.New(CodeFieldNameRequired)
		}
		if f.File && e.BodyType != BodyMultipart {
			return apperr.New(apperr.InvalidInput) // files only travel in multipart
		}
		fields = append(fields, f)
	}

	return d.tx(ctx, func(tx *sql.Tx) error {
		if err := mustAffect(tx.ExecContext(ctx,
			`UPDATE request SET method = ?, url = ?, body_type = ?, body = ?, body_file = ?, post_script = ?,
			   encode_url = ?, follow_redirects = ?, max_redirects = ?, timeout_ms = ?
			 WHERE id = ?`,
			e.Method, e.URL, e.BodyType, e.Body, strings.TrimSpace(e.BodyFile), e.PostScript,
			e.EncodeURL, e.FollowRedirects, e.MaxRedirects, e.TimeoutMS, id)); err != nil {
			return err
		}
		if err := saveVars(ctx, tx, "request_var", "request_id", id, vars); err != nil {
			return err
		}
		if err := saveHeaders(ctx, tx, "request_header", "request_id", id, headers); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM request_body_field WHERE request_id = ?`, id); err != nil {
			return err
		}
		stmt, err := tx.PrepareContext(ctx,
			`INSERT INTO request_body_field (request_id, seq, key, kind, value, content_type, enabled) VALUES (?, ?, ?, ?, ?, ?, ?)`)
		if err != nil {
			return err
		}
		defer stmt.Close()
		for i, f := range fields {
			kind := "text"
			if f.File {
				kind = "file"
			}
			if _, err := stmt.ExecContext(ctx, id, i, f.Key, kind, f.Value, f.ContentType, f.Enabled); err != nil {
				return err
			}
		}
		return nil
	})
}

// FolderEdit is what the folder editor saves.
type FolderEdit struct {
	Vars    []Var
	Headers []Header
}

// LoadFolderEdit returns the variables and headers of a folder.
func (d *DB) LoadFolderEdit(ctx context.Context, id int64) (FolderEdit, error) {
	if _, err := folderRow(ctx, d.db, id); err != nil {
		return FolderEdit{}, err
	}
	var e FolderEdit
	var err error
	if e.Vars, err = loadVars(ctx, d.db, `SELECT key, value, enabled FROM folder_var WHERE folder_id = ? ORDER BY seq, id`, id); err != nil {
		return FolderEdit{}, err
	}
	e.Headers, err = loadHeaders(ctx, d.db, `SELECT key, value, mode, enabled FROM folder_header WHERE folder_id = ? ORDER BY seq, id`, id)
	return e, err
}

// SaveFolderEdit replaces the variables and headers of a folder.
func (d *DB) SaveFolderEdit(ctx context.Context, id int64, e FolderEdit) error {
	vars, err := cleanVars(e.Vars)
	if err != nil {
		return err
	}
	headers, err := cleanHeaders(e.Headers)
	if err != nil {
		return err
	}
	return d.tx(ctx, func(tx *sql.Tx) error {
		if err := exists(ctx, tx, `SELECT 1 FROM folder WHERE id = ?`, id); err != nil {
			return err
		}
		if err := saveVars(ctx, tx, "folder_var", "folder_id", id, vars); err != nil {
			return err
		}
		return saveHeaders(ctx, tx, "folder_header", "folder_id", id, headers)
	})
}

// cleanVars trims keys, drops blank rows and refuses a key outside [\w.-]+.
func cleanVars(vars []Var) ([]Var, error) {
	out := make([]Var, 0, len(vars))
	for _, v := range vars {
		v.Key = strings.TrimSpace(v.Key)
		if v.Key == "" && v.Value == "" {
			continue
		}
		if !varKey.MatchString(v.Key) {
			return nil, apperr.New(CodeInvalidVarKey).With("key", v.Key)
		}
		out = append(out, v)
	}
	return out, nil
}

// cleanHeaders trims names, drops blank rows and refuses a value without a
// name. Names may hold {{variables}}: they are interpolated before sending.
// An ignore row keeps no value.
func cleanHeaders(headers []Header) ([]Header, error) {
	out := make([]Header, 0, len(headers))
	for _, h := range headers {
		h.Key = strings.TrimSpace(h.Key)
		if h.Ignore {
			h.Value = ""
		}
		if h.Key == "" && h.Value == "" {
			continue
		}
		if h.Key == "" {
			return nil, apperr.New(CodeHeaderNameRequired)
		}
		out = append(out, h)
	}
	return out, nil
}

func loadVars(ctx context.Context, db *sql.DB, query string, id int64) ([]Var, error) {
	rows, err := db.QueryContext(ctx, query, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []Var{}
	for rows.Next() {
		var v Var
		if err := rows.Scan(&v.Key, &v.Value, &v.Enabled); err != nil {
			return nil, err
		}
		list = append(list, v)
	}
	return list, rows.Err()
}

func loadHeaders(ctx context.Context, db *sql.DB, query string, id int64) ([]Header, error) {
	rows, err := db.QueryContext(ctx, query, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []Header{}
	for rows.Next() {
		var h Header
		var mode string
		if err := rows.Scan(&h.Key, &h.Value, &mode, &h.Enabled); err != nil {
			return nil, err
		}
		h.Ignore = mode == "ignore"
		list = append(list, h)
	}
	return list, rows.Err()
}

// saveVars replaces the variables of an owner. table and owner are constants.
func saveVars(ctx context.Context, tx *sql.Tx, table, owner string, id int64, vars []Var) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM `+table+` WHERE `+owner+` = ?`, id); err != nil {
		return err
	}
	stmt, err := tx.PrepareContext(ctx, `INSERT INTO `+table+` (`+owner+`, seq, key, value, enabled) VALUES (?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for i, v := range vars {
		if _, err := stmt.ExecContext(ctx, id, i, v.Key, v.Value, v.Enabled); err != nil {
			return err
		}
	}
	return nil
}

// saveHeaders replaces the headers of an owner. table and owner are constants.
func saveHeaders(ctx context.Context, tx *sql.Tx, table, owner string, id int64, headers []Header) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM `+table+` WHERE `+owner+` = ?`, id); err != nil {
		return err
	}
	stmt, err := tx.PrepareContext(ctx, `INSERT INTO `+table+` (`+owner+`, seq, key, value, mode, enabled) VALUES (?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for i, h := range headers {
		mode := "set"
		if h.Ignore {
			mode = "ignore"
		}
		if _, err := stmt.ExecContext(ctx, id, i, h.Key, h.Value, mode, h.Enabled); err != nil {
			return err
		}
	}
	return nil
}
