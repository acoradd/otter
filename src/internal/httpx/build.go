package httpx

import (
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/acoradd/otter/src/internal/apperr"
	"github.com/acoradd/otter/src/internal/resolve"
)

// built is a request ready to go, and what its body is made of.
type built struct {
	req   *http.Request
	parts []SentPart
	// close releases open files when the send is over.
	close func()
}

// buildRequest makes the HTTP request from the preview only.
func buildRequest(ctx context.Context, p resolve.Preview, s Settings) (*built, error) {
	u, err := requestURL(p.URL, s.EncodeURL)
	if err != nil {
		return nil, err
	}
	b := &built{close: func() {}}
	body, length, contentType, err := b.body(p)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, p.Method, "http://placeholder/", body)
	if err != nil {
		b.close()
		return nil, apperr.Wrap(CodeBadURL, err)
	}
	req.URL = u
	req.Host = ""
	req.ContentLength = length
	if body == nil {
		req.Body = nil
	}
	if length < 0 && body != nil {
		req.ContentLength = -1 // streamed: chunked
	}

	// Exactly the headers of the preview: never one Go adds on its own.
	req.Header = http.Header{}
	userAgent := false
	for _, h := range p.Headers {
		if h.Ignored || h.ReadOnly {
			continue
		}
		v := h.Value
		if h.Computed && strings.EqualFold(h.Key, "Content-Type") && contentType != "" {
			v = contentType // the multipart boundary
		}
		if strings.EqualFold(h.Key, "Host") {
			req.Host = v
			continue
		}
		if strings.EqualFold(h.Key, "User-Agent") {
			userAgent = true
		}
		req.Header[textproto.CanonicalMIMEHeaderKey(h.Key)] = append(req.Header[textproto.CanonicalMIMEHeaderKey(h.Key)], v)
	}
	if !userAgent {
		req.Header["User-Agent"] = []string{""} // an empty value: Go sends none
	}
	b.req = req
	return b, nil
}

// requestURL parses the interpolated URL. Encoded, the query is re-encoded
// parameter by parameter, in order. Not encoded, the path and query are sent
// exactly as written.
func requestURL(raw string, encode bool) (*url.URL, error) {
	raw = strings.TrimSpace(raw)
	scheme, rest, ok := strings.Cut(raw, "://")
	if !ok || (!strings.EqualFold(scheme, "http") && !strings.EqualFold(scheme, "https")) {
		return nil, apperr.New(CodeBadURL).With("url", raw)
	}
	if i := strings.IndexByte(rest, '#'); i >= 0 {
		rest = rest[:i]
	}
	host, target := rest, "/"
	if i := strings.IndexAny(rest, "/?"); i >= 0 {
		host, target = rest[:i], rest[i:]
	}
	if host == "" {
		return nil, apperr.New(CodeBadURL).With("url", raw)
	}
	if !strings.HasPrefix(target, "/") {
		target = "/" + target
	}
	path, query, _ := strings.Cut(target, "?")
	u := &url.URL{Scheme: strings.ToLower(scheme), Host: host}

	if !encode {
		if strings.HasPrefix(path, "//") {
			// An opaque path may not start with //: keep it as the full form.
			path = "//" + host + path
		}
		u.Opaque = path
		u.RawQuery = query
		u.ForceQuery = strings.Contains(target, "?") && query == ""
		return u, nil
	}

	parsed, err := url.Parse(u.Scheme + "://" + host + path)
	if err != nil {
		return nil, apperr.Wrap(CodeBadURL, err).With("url", raw)
	}
	u.Path, u.RawPath = parsed.Path, parsed.RawPath
	u.RawQuery = encodeQuery(query)
	u.ForceQuery = strings.Contains(target, "?") && query == ""
	return u, nil
}

// encodeQuery re-encodes each name and value, keeping their order and
// valueless parameters ("?debug").
func encodeQuery(q string) string {
	if q == "" {
		return ""
	}
	parts := strings.Split(q, "&")
	for i, p := range parts {
		k, v, hasValue := strings.Cut(p, "=")
		k = escape(k)
		if hasValue {
			parts[i] = k + "=" + escape(v)
		} else {
			parts[i] = k
		}
	}
	return strings.Join(parts, "&")
}

func escape(s string) string {
	if u, err := url.QueryUnescape(s); err == nil {
		s = u
	}
	return url.QueryEscape(s)
}

// body returns the body reader, its length (-1: unknown, streamed) and, for
// multipart, the Content-Type with its boundary.
func (b *built) body(p resolve.Preview) (io.Reader, int64, string, error) {
	switch p.BodyType {
	case "json", "xml", "text":
		return strings.NewReader(p.Body), int64(len(p.Body)), "", nil
	case "form-urlencoded":
		form := resolve.EncodeForm(p.BodyFields)
		return strings.NewReader(form), int64(len(form)), "", nil
	case "binary":
		if strings.TrimSpace(p.BodyFile) == "" {
			return nil, 0, "", nil
		}
		f, err := os.Open(p.BodyFile)
		if err != nil {
			return nil, 0, "", apperr.Wrap(CodeFileNotFound, err).With("path", p.BodyFile)
		}
		info, err := f.Stat()
		if err != nil {
			f.Close()
			return nil, 0, "", apperr.Wrap(CodeFileNotFound, err).With("path", p.BodyFile)
		}
		b.close = func() { f.Close() }
		b.parts = []SentPart{{Name: filepath.Base(p.BodyFile), File: p.BodyFile, Size: info.Size()}}
		return f, info.Size(), "", nil
	case "multipart":
		return b.multipart(p.BodyFields)
	}
	return nil, 0, "", nil
}

// multipart streams the parts through a pipe: files are never loaded whole.
// A file that cannot be opened is left out (the preview warns about it).
func (b *built) multipart(fields []resolve.ResolvedField) (io.Reader, int64, string, error) {
	var files []*os.File
	type part struct {
		field resolve.ResolvedField
		file  *os.File
	}
	var parts []part
	for _, f := range fields {
		if !f.File {
			parts = append(parts, part{field: f})
			b.parts = append(b.parts, SentPart{Name: f.Key})
			continue
		}
		file, err := os.Open(f.Value)
		if err != nil {
			continue
		}
		size := int64(0)
		if info, err := file.Stat(); err == nil {
			size = info.Size()
		}
		files = append(files, file)
		parts = append(parts, part{field: f, file: file})
		b.parts = append(b.parts, SentPart{Name: f.Key, File: f.Value, Size: size})
	}
	b.close = func() {
		for _, f := range files {
			f.Close()
		}
	}

	pr, pw := io.Pipe()
	mw := multipart.NewWriter(pw)
	go func() {
		err := func() error {
			for _, p := range parts {
				h := textproto.MIMEHeader{}
				if p.file == nil {
					h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="%s"`, quote(p.field.Key)))
					if p.field.ContentType != "" {
						h.Set("Content-Type", p.field.ContentType)
					}
					w, err := mw.CreatePart(h)
					if err != nil {
						return err
					}
					if _, err := io.WriteString(w, p.field.Value); err != nil {
						return err
					}
					continue
				}
				h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="%s"; filename="%s"`, quote(p.field.Key), quote(filepath.Base(p.field.Value))))
				ct := p.field.ContentType
				if ct == "" {
					ct = resolve.FileContentType(p.field.Value)
				}
				h.Set("Content-Type", ct)
				w, err := mw.CreatePart(h)
				if err != nil {
					return err
				}
				if _, err := io.Copy(w, p.file); err != nil {
					return err
				}
			}
			return mw.Close()
		}()
		pw.CloseWithError(err)
	}()
	return pr, -1, mw.FormDataContentType(), nil
}

var quoteEscaper = strings.NewReplacer("\\", "\\\\", `"`, "\\\"")

func quote(s string) string { return quoteEscaper.Replace(s) }

// BodyText is the body as sent, for the history: text bodies as they are,
// form fields encoded; files are described by name and size instead.
func BodyText(p resolve.Preview) string {
	switch p.BodyType {
	case "json", "xml", "text":
		return p.Body
	case "form-urlencoded":
		return resolve.EncodeForm(p.BodyFields)
	}
	return ""
}
