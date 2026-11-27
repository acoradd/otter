package httpx

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"compress/zlib"
	"context"
	"encoding/pem"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/andybalholm/brotli"

	"github.com/acoradd/otter/src/internal/apperr"
	"github.com/acoradd/otter/src/internal/resolve"
)

var ctx = context.Background()

// preview resolves a request the way the app does: Send only sees this.
func preview(r resolve.Request) resolve.Preview {
	if r.Method == "" {
		r.Method = "GET"
	}
	if r.BodyType == "" {
		r.BodyType = "none"
	}
	return resolve.Resolve(resolve.Input{Request: &r, OS: "linux"})
}

// direct sends without the sandbox's or the system's proxy.
func direct(t *testing.T, o Options) Options {
	t.Helper()
	tr, err := NewTransport(o.Network)
	if err != nil {
		t.Fatal(err)
	}
	tr.Proxy = nil
	t.Cleanup(tr.CloseIdleConnections)
	o.Transport = tr
	if o.Settings.TimeoutMS == 0 {
		o.Settings.TimeoutMS = 5000
	}
	return o
}

func defaults() Settings {
	return Settings{EncodeURL: true, FollowRedirects: true, MaxRedirects: 10, TimeoutMS: 5000}
}

func wantCode(t *testing.T, err error, code apperr.Code) {
	t.Helper()
	var e *apperr.Error
	if !errors.As(err, &e) || e.Code != code {
		t.Fatalf("error = %v, want %s", err, code)
	}
}

// received records what the server got.
type received struct {
	method, requestURI, host string
	header                   http.Header
	body                     []byte
	contentLength            int64
}

func recorder(t *testing.T, reply func(w http.ResponseWriter, r *http.Request)) (*httptest.Server, *received) {
	t.Helper()
	got := &received{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.method, got.requestURI, got.host, got.header = r.Method, r.RequestURI, r.Host, r.Header.Clone()
		got.contentLength = r.ContentLength
		got.body, _ = io.ReadAll(r.Body)
		if reply != nil {
			reply(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, got
}

func TestSendsExactlyThePreviewHeaders(t *testing.T) {
	srv, got := recorder(t, nil)
	p := preview(resolve.Request{
		Method: "patch", URL: srv.URL + "/users/1",
		Headers: []resolve.Header{
			{Key: "X-Trace", Value: "abc", Enabled: true},
			{Key: "x-multi", Value: "1", Enabled: true},
			{Key: "Accept", Ignore: true, Enabled: true},
			{Key: "User-Agent", Ignore: true, Enabled: true},
			{Key: "Host", Value: "virtual.test", Enabled: true},
		},
	})
	res, err := Send(ctx, p, direct(t, Options{Settings: defaults()}))
	if err != nil {
		t.Fatal(err)
	}
	if got.method != "PATCH" || got.host != "virtual.test" {
		t.Errorf("method %s host %s", got.method, got.host)
	}
	if got.header.Get("X-Trace") != "abc" || got.header.Get("X-Multi") != "1" || got.header.Get("Accept-Encoding") != "gzip, deflate, br" {
		t.Errorf("headers = %v", got.header)
	}
	if _, ok := got.header["Accept"]; ok {
		t.Error("an ignored header must not be sent")
	}
	if ua, ok := got.header["User-Agent"]; ok {
		t.Errorf("User-Agent ignored, yet Go sent %q", ua)
	}
	if res.Status != 200 || res.StatusText != "OK" || res.Proto != "HTTP/1.1" {
		t.Errorf("response = %d %q %s", res.Status, res.StatusText, res.Proto)
	}
	for _, h := range res.SentHeaders {
		if h.Key == "User-Agent" {
			t.Errorf("sent headers list a User-Agent: %+v", h)
		}
	}
	if res.SentHeaders[0] != (Header{Key: "Host", Value: "virtual.test"}) || res.Method != "PATCH" {
		t.Errorf("sent: %s %+v", res.Method, res.SentHeaders[0])
	}
}

func TestAutomaticUserAgent(t *testing.T) {
	srv, got := recorder(t, nil)
	if _, err := Send(ctx, preview(resolve.Request{URL: srv.URL}), direct(t, Options{Settings: defaults()})); err != nil {
		t.Fatal(err)
	}
	if got.header.Get("User-Agent") != resolve.UserAgent("linux") {
		t.Errorf("User-Agent = %q", got.header.Get("User-Agent"))
	}
}

func TestURLEncoding(t *testing.T) {
	tests := []struct {
		name   string
		path   string
		encode bool
		want   string
	}{
		{"raw: byte for byte", "/a%2fb/%7Euser/./x?q=a+b&r=%2F&empty&z=%41", false, "/a%2fb/%7Euser/./x?q=a+b&r=%2F&empty&z=%41"},
		{"raw: an empty query stays", "/x?", false, "/x?"},
		{"raw: no path", "", false, "/"},
		{"encoded: spaces and accents", "/users/ä b?q=a b&tag=x%20y&flag", true, "/users/%C3%A4%20b?q=a+b&tag=x+y&flag"},
		{"encoded: order kept", "/s?z=1&a=2&m=3", true, "/s?z=1&a=2&m=3"},
		{"encoded: reserved characters in values", "/s?redirect=https://x.test/a?b=c&d", true, "/s?redirect=https%3A%2F%2Fx.test%2Fa%3Fb%3Dc&d"},
		{"fragment dropped", "/page#top", true, "/page"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv, got := recorder(t, nil)
			s := defaults()
			s.EncodeURL = tt.encode
			if _, err := Send(ctx, preview(resolve.Request{URL: srv.URL + tt.path}), direct(t, Options{Settings: s})); err != nil {
				t.Fatal(err)
			}
			if got.requestURI != tt.want {
				t.Errorf("request target = %q, want %q", got.requestURI, tt.want)
			}
		})
	}
	for _, bad := range []string{"", "ftp://x.test/a", "https:///nohost", "localhost:8080/x"} {
		_, err := Send(ctx, preview(resolve.Request{URL: bad}), direct(t, Options{Settings: defaults()}))
		wantCode(t, err, CodeBadURL)
	}
}

func TestBodies(t *testing.T) {
	dir := t.TempDir()
	avatar := filepath.Join(dir, "avatar.png")
	if err := os.WriteFile(avatar, []byte("PNG-DATA"), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Run("json", func(t *testing.T) {
		srv, got := recorder(t, nil)
		p := preview(resolve.Request{Method: "POST", URL: srv.URL, BodyType: "json", Body: `{"name":"Ada"}`})
		if _, err := Send(ctx, p, direct(t, Options{Settings: defaults()})); err != nil {
			t.Fatal(err)
		}
		if string(got.body) != `{"name":"Ada"}` || got.contentLength != 14 || got.header.Get("Content-Type") != "application/json" {
			t.Errorf("body %q length %d type %q", got.body, got.contentLength, got.header.Get("Content-Type"))
		}
	})

	t.Run("form", func(t *testing.T) {
		srv, got := recorder(t, nil)
		p := preview(resolve.Request{Method: "POST", URL: srv.URL, BodyType: "form-urlencoded", Fields: []resolve.Field{
			{Key: "z", Value: "a b&c", Enabled: true}, {Key: "a", Value: "é", Enabled: true}, {Key: "off", Value: "x"},
		}})
		if _, err := Send(ctx, p, direct(t, Options{Settings: defaults()})); err != nil {
			t.Fatal(err)
		}
		if string(got.body) != "z=a+b%26c&a=%C3%A9" {
			t.Errorf("form = %q", got.body)
		}
	})

	t.Run("multipart, streamed, a missing file left out", func(t *testing.T) {
		srv, got := recorder(t, nil)
		p := preview(resolve.Request{Method: "POST", URL: srv.URL, BodyType: "multipart", Fields: []resolve.Field{
			{Key: "name", Value: "Ada", Enabled: true},
			{Key: "avatar", Value: avatar, File: true, Enabled: true},
			{Key: "gone", Value: filepath.Join(dir, "nope.bin"), File: true, Enabled: true},
			{Key: "meta", Value: `{"a":1}`, ContentType: "application/json", Enabled: true},
		}})
		res, err := Send(ctx, p, direct(t, Options{Settings: defaults()}))
		if err != nil {
			t.Fatal(err)
		}
		mediaType, params, _ := mime.ParseMediaType(got.header.Get("Content-Type"))
		if mediaType != "multipart/form-data" || params["boundary"] == "" {
			t.Fatalf("Content-Type = %q", got.header.Get("Content-Type"))
		}
		if got.contentLength != -1 {
			t.Errorf("a multipart body is streamed (chunked), length %d", got.contentLength)
		}
		mr := multipart.NewReader(bytes.NewReader(got.body), params["boundary"])
		var parts []string
		for {
			part, err := mr.NextPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Fatal(err)
			}
			data, _ := io.ReadAll(part)
			parts = append(parts, part.FormName()+"|"+part.FileName()+"|"+part.Header.Get("Content-Type")+"|"+string(data))
		}
		want := []string{"name|||Ada", "avatar|avatar.png|image/png|PNG-DATA", `meta||application/json|{"a":1}`}
		if !reflect.DeepEqual(parts, want) {
			t.Errorf("parts = %q\nwant %q", parts, want)
		}
		wantSent := []SentPart{{Name: "name"}, {Name: "avatar", File: avatar, Size: 8}, {Name: "meta"}}
		if !reflect.DeepEqual(res.SentParts, wantSent) {
			t.Errorf("sent parts = %+v", res.SentParts)
		}
	})

	t.Run("file, streamed with its length", func(t *testing.T) {
		srv, got := recorder(t, nil)
		p := preview(resolve.Request{Method: "PUT", URL: srv.URL, BodyType: "binary", BodyFile: avatar})
		res, err := Send(ctx, p, direct(t, Options{Settings: defaults()}))
		if err != nil {
			t.Fatal(err)
		}
		if string(got.body) != "PNG-DATA" || got.contentLength != 8 || got.header.Get("Content-Type") != "image/png" {
			t.Errorf("body %q length %d type %q", got.body, got.contentLength, got.header.Get("Content-Type"))
		}
		if len(res.SentParts) != 1 || res.SentParts[0].Size != 8 {
			t.Errorf("sent parts = %+v", res.SentParts)
		}
		p = preview(resolve.Request{Method: "PUT", URL: srv.URL, BodyType: "binary", BodyFile: filepath.Join(dir, "gone.bin")})
		_, err = Send(ctx, p, direct(t, Options{Settings: defaults()}))
		wantCode(t, err, CodeFileNotFound)
	})
}

func TestRedirects(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/a":
			http.Redirect(w, r, "/b", http.StatusFound)
		case "/b":
			http.Redirect(w, r, srv.URL+"/c", http.StatusMovedPermanently)
		case "/loop":
			http.Redirect(w, r, "/loop", http.StatusFound)
		default:
			w.Header().Set("X-UA", r.Header.Get("User-Agent"))
			io.WriteString(w, "done")
		}
	}))
	t.Cleanup(srv.Close)

	res, err := Send(ctx, preview(resolve.Request{URL: srv.URL + "/a"}), direct(t, Options{Settings: defaults()}))
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != 200 || string(res.Body) != "done" || res.URL != srv.URL+"/c" {
		t.Fatalf("final = %d %q %s", res.Status, res.Body, res.URL)
	}
	post, err := Send(ctx, preview(resolve.Request{Method: "POST", URL: srv.URL + "/a", BodyType: "text", Body: "x"}), direct(t, Options{Settings: defaults()}))
	if err != nil || post.Method != "GET" {
		t.Errorf("after a 302, a POST goes on as a GET, and says so: %q %v", post.Method, err)
	}
	want := []Redirect{{302, srv.URL + "/a", "/b"}, {301, srv.URL + "/b", srv.URL + "/c"}}
	if !reflect.DeepEqual(res.Redirects, want) {
		t.Errorf("chain = %+v", res.Redirects)
	}

	// The automatic User-Agent follows; an ignored one stays away on every hop.
	p := preview(resolve.Request{URL: srv.URL + "/a", Headers: []resolve.Header{{Key: "User-Agent", Ignore: true, Enabled: true}}})
	res, _ = Send(ctx, p, direct(t, Options{Settings: defaults()}))
	if ua := headerValue(res.Headers, "X-Ua"); ua != "" {
		t.Errorf("Go's User-Agent appeared after a redirect: %q", ua)
	}

	s := defaults()
	s.FollowRedirects = false
	res, err = Send(ctx, preview(resolve.Request{URL: srv.URL + "/a"}), direct(t, Options{Settings: s}))
	if err != nil || res.Status != 302 || len(res.Redirects) != 0 || headerValue(res.Headers, "Location") != "/b" {
		t.Fatalf("not followed: %+v, %v", res, err)
	}

	s = defaults()
	s.MaxRedirects = 3
	_, err = Send(ctx, preview(resolve.Request{URL: srv.URL + "/loop"}), direct(t, Options{Settings: s}))
	wantCode(t, err, CodeTooManyRedirects)
}

func headerValue(list []Header, key string) string {
	for _, h := range list {
		if strings.EqualFold(h.Key, key) {
			return h.Value
		}
	}
	return ""
}

func TestDecompression(t *testing.T) {
	text := strings.Repeat("Otter decompresses this. ", 40)
	compress := map[string]func() []byte{
		"gzip": func() []byte {
			var b bytes.Buffer
			w := gzip.NewWriter(&b)
			w.Write([]byte(text))
			w.Close()
			return b.Bytes()
		},
		"deflate": func() []byte { // zlib-wrapped, as the RFC says
			var b bytes.Buffer
			w := zlib.NewWriter(&b)
			w.Write([]byte(text))
			w.Close()
			return b.Bytes()
		},
		"deflate-raw": func() []byte { // raw, as many servers send it
			var b bytes.Buffer
			w, _ := flate.NewWriter(&b, flate.BestCompression)
			w.Write([]byte(text))
			w.Close()
			return b.Bytes()
		},
		"br": func() []byte {
			var b bytes.Buffer
			w := brotli.NewWriter(&b)
			w.Write([]byte(text))
			w.Close()
			return b.Bytes()
		},
	}
	for name, make := range compress {
		t.Run(name, func(t *testing.T) {
			wire := make()
			encoding := strings.TrimSuffix(name, "-raw")
			srv, _ := recorder(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Encoding", encoding)
				w.Write(wire)
			})
			res, err := Send(ctx, preview(resolve.Request{URL: srv.URL}), direct(t, Options{Settings: defaults()}))
			if err != nil {
				t.Fatal(err)
			}
			if string(res.Body) != text || res.Size != int64(len(text)) || res.WireSize != int64(len(wire)) || res.Encoding != encoding {
				t.Errorf("body %d bytes (wire %d, want %d), encoding %q", res.Size, res.WireSize, len(wire), res.Encoding)
			}
		})
	}

	srv, _ := recorder(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Encoding", "gzip")
		w.Write([]byte("not gzip at all"))
	})
	_, err := Send(ctx, preview(resolve.Request{URL: srv.URL}), direct(t, Options{Settings: defaults()}))
	wantCode(t, err, CodeDecompress)

	srv, _ = recorder(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Encoding", "zstd")
		w.Write([]byte("left as is"))
	})
	res, err := Send(ctx, preview(resolve.Request{URL: srv.URL}), direct(t, Options{Settings: defaults()}))
	if err != nil || string(res.Body) != "left as is" || res.Encoding != "" {
		t.Errorf("unknown encoding: %q %q %v", res.Body, res.Encoding, err)
	}
}

func TestTimeoutCancelAndDuration(t *testing.T) {
	release := make(chan struct{})
	srv, _ := recorder(t, func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
	})
	t.Cleanup(func() { close(release) })

	s := defaults()
	s.TimeoutMS = 50
	_, err := Send(ctx, preview(resolve.Request{URL: srv.URL}), direct(t, Options{Settings: s}))
	wantCode(t, err, CodeTimeout)

	cctx, cancel := context.WithCancel(ctx)
	go func() { time.Sleep(30 * time.Millisecond); cancel() }()
	_, err = Send(cctx, preview(resolve.Request{URL: srv.URL}), direct(t, Options{Settings: defaults()}))
	wantCode(t, err, CodeCancelled)

	fast, _ := recorder(t, nil)
	t0 := time.Date(2026, 11, 27, 10, 0, 0, 0, time.UTC)
	calls := 0
	clock := func() time.Time { calls++; return t0.Add(time.Duration(calls-1) * 125 * time.Millisecond) }
	res, err := Send(ctx, preview(resolve.Request{URL: fast.URL}), direct(t, Options{Settings: defaults(), Now: clock}))
	if err != nil || res.DurationMS != 125 {
		t.Errorf("duration = %d, %v", res.DurationMS, err)
	}
}

func TestConnectionErrors(t *testing.T) {
	srv, _ := recorder(t, nil)
	url := srv.URL
	srv.Close()
	_, err := Send(ctx, preview(resolve.Request{URL: url}), direct(t, Options{Settings: defaults()}))
	wantCode(t, err, CodeConnRefused)

	_, err = Send(ctx, preview(resolve.Request{URL: "http://otter-test.invalid/"}), direct(t, Options{Settings: defaults()}))
	wantCode(t, err, CodeHostNotFound)
}

func TestCookies(t *testing.T) {
	srv, got := recorder(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/login" {
			http.SetCookie(w, &http.Cookie{Name: "session", Value: "abc", Path: "/"})
		}
	})
	jar := NewJar()
	o := direct(t, Options{Settings: defaults(), Jar: jar})
	if _, err := Send(ctx, preview(resolve.Request{URL: srv.URL + "/login"}), o); err != nil {
		t.Fatal(err)
	}
	if _, err := Send(ctx, preview(resolve.Request{URL: srv.URL + "/me"}), o); err != nil {
		t.Fatal(err)
	}
	if got.header.Get("Cookie") != "session=abc" {
		t.Errorf("Cookie = %q", got.header.Get("Cookie"))
	}
	o.Jar = nil
	Send(ctx, preview(resolve.Request{URL: srv.URL + "/me"}), o)
	if got.header.Get("Cookie") != "" {
		t.Errorf("without a jar, no cookie: %q", got.header.Get("Cookie"))
	}
}

func TestTLS(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "secure") }))
	t.Cleanup(srv.Close)
	p := preview(resolve.Request{URL: srv.URL})

	_, err := Send(ctx, p, direct(t, Options{Settings: defaults()}))
	wantCode(t, err, CodeTLS)

	res, err := Send(ctx, p, direct(t, Options{Settings: defaults(), Network: Network{TLSInsecure: true}}))
	if err != nil || string(res.Body) != "secure" {
		t.Fatalf("insecure: %v", err)
	}

	ca := filepath.Join(t.TempDir(), "ca.pem")
	pemData := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw})
	if err := os.WriteFile(ca, pemData, 0o644); err != nil {
		t.Fatal(err)
	}
	res, err = Send(ctx, p, direct(t, Options{Settings: defaults(), Network: Network{CACertPath: ca}}))
	if err != nil || string(res.Body) != "secure" {
		t.Fatalf("with the server's CA: %v", err)
	}
}

func TestNetworkSettingsRefused(t *testing.T) {
	tests := []struct {
		name string
		n    Network
		want apperr.Code
	}{
		{"proxy without a host", Network{ProxyURL: "not a url"}, CodeProxy},
		{"missing CA file", Network{CACertPath: "/nowhere/ca.pem"}, CodeCertificate},
		{"CA file without certificates", Network{CACertPath: writeTemp(t, "not pem")}, CodeCertificate},
		{"missing client certificate", Network{ClientCertPath: "/nowhere/c.pem", ClientKeyPath: "/nowhere/k.pem"}, CodeCertificate},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewTransport(tt.n)
			wantCode(t, err, tt.want)
		})
	}
	tr, err := NewTransport(Network{ProxyURL: "http://proxy.test:3128"})
	if err != nil || tr.Proxy == nil || !tr.DisableCompression {
		t.Fatalf("transport = %v, %v", tr, err)
	}
}

func writeTemp(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "f")
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestBodyText(t *testing.T) {
	if got := BodyText(preview(resolve.Request{BodyType: "json", Body: "{}", URL: "http://h"})); got != "{}" {
		t.Errorf("json = %q", got)
	}
	if got := BodyText(preview(resolve.Request{BodyType: "binary", BodyFile: "/x", URL: "http://h"})); got != "" {
		t.Errorf("a file is never copied: %q", got)
	}
}
