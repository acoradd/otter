package main

import (
	"context"
	"encoding/json"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/acoradd/otter/src/internal/apperr"
	"github.com/acoradd/otter/src/internal/httpx"
	"github.com/acoradd/otter/src/internal/resolve"
	"github.com/acoradd/otter/src/internal/store"
)

// EventHistoryChanged is emitted after a send, with the request's ID.
const EventHistoryChanged = "history.changed"

const (
	// maxShownBody is the largest body the response view carries as text;
	// past it the user saves it to a file (docs/specs, section 7).
	maxShownBody = 5 << 20
	// maxStoredBody is the largest response body kept in the history.
	maxStoredBody = 1 << 20
	// keptBodies is how many full response bodies stay in memory to be saved.
	keptBodies = 20
	// secretMask replaces secret values in what is recorded.
	secretMask = "••••••"
)

// Body kinds of a response view.
const (
	BodyText   = "text"
	BodyBinary = "binary"
	BodyLarge  = "large"
	BodyEmpty  = "empty"
)

// ResponseView is a response as the response panel shows it.
type ResponseView struct {
	// ID is the history entry: SaveResponseBody takes it.
	ID         int64  `json:"id"`
	SentAt     int64  `json:"sentAt"`
	Method     string `json:"method"`
	Status     int    `json:"status"`
	StatusText string `json:"statusText"`
	// URL is the final URL, after redirects.
	URL        string           `json:"url"`
	DurationMS int64            `json:"durationMs"`
	Size       int64            `json:"size"`
	WireSize   int64            `json:"wireSize"`
	Encoding   string           `json:"encoding"`
	Headers    []httpx.Header   `json:"headers"`
	Redirects  []httpx.Redirect `json:"redirects"`
	// SentHeaders are the request headers as they left, secrets masked.
	SentHeaders []httpx.Header `json:"sentHeaders"`
	// ContentType is the media type of the body, without parameters.
	ContentType string `json:"contentType"`
	// BodyKind is text, binary, large (past 5 MB) or empty. Body holds the
	// text only for "text".
	BodyKind string `json:"bodyKind"`
	Body     string `json:"body"`
	// Saveable: the full body can still be saved to a file.
	Saveable bool `json:"saveable"`
	// Error is the code of a failed send, "" when a response came.
	Error string `json:"error"`
}

// HistoryItem is a send in the history list.
type HistoryItem struct {
	ID         int64  `json:"id"`
	SentAt     int64  `json:"sentAt"`
	Method     string `json:"method"`
	URL        string `json:"url"`
	Status     int    `json:"status"`
	DurationMS int64  `json:"durationMs"`
	Size       int64  `json:"size"`
	Error      string `json:"error"`
}

// SendService sends requests, keeps the history, and saves response bodies.
type SendService struct {
	db      *store.DB
	preview *PreviewService
	emit    func(name string, data any)
	now     func() time.Time
	dialogs Dialogs
	// transport replaces the network in tests; nil builds one per send.
	transport http.RoundTripper

	mu       sync.Mutex
	inflight map[int64]context.CancelFunc
	jars     map[int64]http.CookieJar
	bodies   map[int64][]byte
	order    []int64
}

func NewSendService(db *store.DB, preview *PreviewService, emit func(string, any), now func() time.Time, dialogs Dialogs) *SendService {
	return &SendService{
		db: db, preview: preview, emit: emit, now: now, dialogs: dialogs,
		inflight: map[int64]context.CancelFunc{}, jars: map[int64]http.CookieJar{}, bodies: map[int64][]byte{},
	}
}

// SendRequest sends a request as the editor holds it, saved or not: the
// draft is resolved by the same code path as the preview, and only that
// preview is sent. A failed send is recorded too, and returned as its code.
func (s *SendService) SendRequest(ctx context.Context, requestID int64, d RequestDraft) (ResponseView, error) {
	in, err := s.preview.requestInput(ctx, requestID, d)
	if err != nil {
		return ResponseView{}, err
	}
	r, err := s.db.Request(ctx, requestID)
	if err != nil {
		return ResponseView{}, err
	}
	p := resolve.Resolve(in)

	ctx, cancel := context.WithCancel(ctx)
	s.mu.Lock()
	if previous := s.inflight[requestID]; previous != nil {
		previous() // a new send replaces the running one
	}
	s.inflight[requestID] = cancel
	jar := s.jars[r.WorkspaceID]
	if jar == nil {
		jar = httpx.NewJar()
		s.jars[r.WorkspaceID] = jar
	}
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.inflight, requestID)
		s.mu.Unlock()
		cancel()
	}()

	sentAt := s.now()
	res, sendErr := httpx.Send(ctx, p, httpx.Options{
		Settings: httpx.Settings{
			EncodeURL: d.EncodeURL, FollowRedirects: d.FollowRedirects, MaxRedirects: d.MaxRedirects, TimeoutMS: d.TimeoutMS,
		},
		Jar:       jar,
		Now:       s.now,
		Transport: s.transport,
	})

	entry := store.HistoryEntry{
		RequestID: requestID, WorkspaceID: r.WorkspaceID, SentAt: sentAt.UnixMilli(), Method: p.Method, URL: p.URL,
		ReqBody: httpx.BodyText(p),
	}
	var view ResponseView
	if sendErr != nil {
		entry.Error = string(apperr.CodeOf(sendErr))
		entry.ReqHeaders = toJSON(maskHeaders(previewHeaders(p), p))
	} else {
		view = s.view(res, p)
		entry.Status, entry.DurationMS, entry.SizeBytes = res.Status, res.DurationMS, res.Size
		entry.URL = res.URL
		entry.ReqHeaders = toJSON(view.SentHeaders)
		entry.ResHeaders = toJSON(res.Headers)
		if len(res.SentParts) > 0 {
			entry.ReqBody = toJSON(res.SentParts) // files by name and size, never their content
		}
		if view.BodyKind == BodyText && len(res.Body) <= maxStoredBody {
			entry.ResBody = string(res.Body)
		}
	}
	id, err := s.db.AddHistory(context.WithoutCancel(ctx), entry)
	if err != nil {
		return ResponseView{}, err
	}
	s.emit(EventHistoryChanged, requestID)
	if sendErr != nil {
		return ResponseView{}, sendErr
	}
	view.ID, view.SentAt = id, entry.SentAt
	s.keep(id, res.Body)
	view.Saveable = len(res.Body) > 0
	return view, nil
}

// CancelRequest stops the send of a request, if one is running.
func (s *SendService) CancelRequest(requestID int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if cancel := s.inflight[requestID]; cancel != nil {
		cancel()
	}
}

// ClearCookies forgets the cookies a workspace received.
func (s *SendService) ClearCookies(workspaceID int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.jars, workspaceID)
}

// GetHistory lists the sends of a request, newest first.
func (s *SendService) GetHistory(ctx context.Context, requestID int64) ([]HistoryItem, error) {
	list, err := s.db.History(ctx, requestID)
	if err != nil {
		return nil, err
	}
	out := make([]HistoryItem, len(list))
	for i, e := range list {
		out[i] = HistoryItem{ID: e.ID, SentAt: e.SentAt, Method: e.Method, URL: e.URL, Status: e.Status, DurationMS: e.DurationMS, Size: e.SizeBytes, Error: e.Error}
	}
	return out, nil
}

// GetHistoryResponse returns a recorded send as the response panel shows it.
func (s *SendService) GetHistoryResponse(ctx context.Context, id int64) (ResponseView, error) {
	e, err := s.db.HistoryEntryByID(ctx, id)
	if err != nil {
		return ResponseView{}, err
	}
	v := ResponseView{
		ID: e.ID, SentAt: e.SentAt, Method: e.Method, Status: e.Status, StatusText: http.StatusText(e.Status), URL: e.URL,
		DurationMS: e.DurationMS, Size: e.SizeBytes, WireSize: e.SizeBytes, Error: e.Error,
		Headers: fromJSON(e.ResHeaders), SentHeaders: fromJSON(e.ReqHeaders), Redirects: []httpx.Redirect{},
	}
	v.ContentType = mediaType(v.Headers)
	s.mu.Lock()
	full := s.bodies[id]
	s.mu.Unlock()
	switch {
	case e.Error != "":
		v.BodyKind = BodyEmpty
	case e.ResBody != "":
		v.BodyKind, v.Body = BodyText, e.ResBody
	case e.SizeBytes == 0:
		v.BodyKind = BodyEmpty
	case e.SizeBytes > maxShownBody:
		v.BodyKind = BodyLarge
	default:
		v.BodyKind = BodyBinary
	}
	v.Saveable = full != nil || e.ResBody != ""
	return v, nil
}

// SaveResponseBody asks where to save the body of a response and writes it.
// It returns the path, "" when the user cancelled.
func (s *SendService) SaveResponseBody(ctx context.Context, id int64) (string, error) {
	e, err := s.db.HistoryEntryByID(ctx, id)
	if err != nil {
		return "", err
	}
	s.mu.Lock()
	body := s.bodies[id]
	s.mu.Unlock()
	if body == nil {
		if e.ResBody == "" {
			return "", apperr.New(CodeBodyGone)
		}
		body = []byte(e.ResBody)
	}
	dest, err := s.dialogs.SaveFile("", suggestedName(e.URL, mediaType(fromJSON(e.ResHeaders))))
	if err != nil || dest == "" {
		return "", err
	}
	if err := os.WriteFile(dest, body, 0o644); err != nil {
		return "", apperr.Wrap(CodeWriteFailed, err).With("path", dest)
	}
	return dest, nil
}

// Error codes of saving a body.
const (
	CodeBodyGone    apperr.Code = "BODY_NOT_KEPT"
	CodeWriteFailed apperr.Code = "WRITE_FAILED"
)

// view is the response panel's view of a response.
func (s *SendService) view(res *httpx.Response, p resolve.Preview) ResponseView {
	v := ResponseView{
		Method: res.Method, Status: res.Status, StatusText: res.StatusText, URL: res.URL, DurationMS: res.DurationMS,
		Size: res.Size, WireSize: res.WireSize, Encoding: res.Encoding, Headers: res.Headers, Redirects: res.Redirects,
		SentHeaders: maskHeaders(res.SentHeaders, p),
	}
	v.ContentType = mediaType(res.Headers)
	switch {
	case len(res.Body) == 0:
		v.BodyKind = BodyEmpty
	case !isText(v.ContentType, res.Body):
		v.BodyKind = BodyBinary
	case len(res.Body) > maxShownBody:
		v.BodyKind = BodyLarge
	default:
		v.BodyKind, v.Body = BodyText, string(res.Body)
	}
	return v
}

// keep holds the latest full bodies in memory, for "Save to file".
func (s *SendService) keep(id int64, body []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.bodies[id] = body
	s.order = append(s.order, id)
	for len(s.order) > keptBodies {
		delete(s.bodies, s.order[0])
		s.order = s.order[1:]
	}
}

// maskHeaders hides the values of headers that hold a secret in the preview.
func maskHeaders(headers []httpx.Header, p resolve.Preview) []httpx.Header {
	secret := map[string]bool{}
	for _, h := range p.Headers {
		if h.Secret {
			secret[strings.ToLower(h.Key)] = true
		}
	}
	out := make([]httpx.Header, len(headers))
	for i, h := range headers {
		if secret[strings.ToLower(h.Key)] {
			h.Value = secretMask
		}
		out[i] = h
	}
	return out
}

// previewHeaders are the headers a failed send meant to send.
func previewHeaders(p resolve.Preview) []httpx.Header {
	out := []httpx.Header{}
	for _, h := range p.Headers {
		if !h.Ignored && !h.Computed {
			out = append(out, httpx.Header{Key: h.Key, Value: h.Value})
		}
	}
	return out
}

func mediaType(headers []httpx.Header) string {
	for _, h := range headers {
		if strings.EqualFold(h.Key, "Content-Type") {
			if t, _, err := mime.ParseMediaType(h.Value); err == nil {
				return t
			}
			return strings.TrimSpace(strings.SplitN(h.Value, ";", 2)[0])
		}
	}
	return ""
}

// isText tells a body the editor can show: a text media type, or valid
// UTF-8 without NUL bytes when the type says nothing.
func isText(contentType string, body []byte) bool {
	t := strings.ToLower(contentType)
	switch {
	case strings.HasPrefix(t, "text/"), strings.HasSuffix(t, "+json"), strings.HasSuffix(t, "+xml"),
		t == "application/json", t == "application/xml", t == "application/javascript",
		t == "application/x-www-form-urlencoded", t == "application/yaml", t == "application/problem+json":
		return utf8.Valid(body)
	case strings.HasPrefix(t, "image/"), strings.HasPrefix(t, "audio/"), strings.HasPrefix(t, "video/"),
		t == "application/octet-stream", t == "application/pdf", t == "application/zip", t == "application/gzip":
		return false
	}
	return utf8.Valid(body) && !strings.ContainsRune(string(body), 0)
}

// suggestedName is a file name for a body: the URL's last segment, or
// "response" with an extension from the media type.
func suggestedName(rawURL, contentType string) string {
	if u, err := url.Parse(rawURL); err == nil {
		if base := path.Base(u.Path); base != "" && base != "/" && base != "." && strings.Contains(base, ".") {
			return base
		}
	}
	ext := ".bin"
	switch {
	case strings.Contains(contentType, "json"):
		ext = ".json"
	case strings.Contains(contentType, "xml"):
		ext = ".xml"
	case strings.Contains(contentType, "html"):
		ext = ".html"
	case strings.HasPrefix(contentType, "text/"):
		ext = ".txt"
	default:
		if exts, _ := mime.ExtensionsByType(contentType); len(exts) > 0 {
			ext = exts[0]
		}
	}
	return "response" + ext
}

func toJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func fromJSON(s string) []httpx.Header {
	out := []httpx.Header{}
	if s != "" {
		_ = json.Unmarshal([]byte(s), &out)
	}
	return out
}
