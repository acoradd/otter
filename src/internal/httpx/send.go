package httpx

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"compress/zlib"
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/andybalholm/brotli"

	"github.com/acoradd/otter/src/internal/apperr"
	"github.com/acoradd/otter/src/internal/resolve"
)

// Options are what a send needs besides the preview.
type Options struct {
	Settings Settings
	Network  Network
	// Jar keeps the workspace's cookies; nil sends none and keeps none.
	Jar http.CookieJar
	// Now reads the clock (time.Now), for the duration.
	Now func() time.Time
	// Transport replaces the network in tests; nil builds one from Network.
	Transport http.RoundTripper
}

// NewJar is an in-memory cookie jar for a workspace.
func NewJar() http.CookieJar {
	jar, _ := cookiejar.New(nil)
	return jar
}

// Send sends the request of the preview and reads the whole response.
// Cancelling ctx stops it (SEND_CANCELLED); the request's timeout covers
// everything, body included (SEND_TIMEOUT).
func Send(ctx context.Context, p resolve.Preview, o Options) (*Response, error) {
	if o.Now == nil {
		o.Now = time.Now
	}
	if o.Settings.TimeoutMS > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(o.Settings.TimeoutMS)*time.Millisecond)
		defer cancel()
	}
	b, err := buildRequest(ctx, p, o.Settings)
	if err != nil {
		return nil, err
	}
	defer b.close()

	transport := o.Transport
	if transport == nil {
		t, err := NewTransport(o.Network)
		if err != nil {
			return nil, err
		}
		defer t.CloseIdleConnections()
		transport = t
	}
	res := &Response{Redirects: []Redirect{}, SentParts: b.parts}
	if res.SentParts == nil {
		res.SentParts = []SentPart{}
	}
	client := &http.Client{
		Transport: transport,
		Jar:       o.Jar,
		CheckRedirect: func(next *http.Request, via []*http.Request) error {
			if !o.Settings.FollowRedirects {
				return http.ErrUseLastResponse
			}
			prev := next.Response
			res.Redirects = append(res.Redirects, Redirect{
				Status: prev.StatusCode, URL: prev.Request.URL.String(), Location: prev.Header.Get("Location"),
			})
			if len(via) > o.Settings.MaxRedirects {
				return apperr.New(CodeTooManyRedirects).With("max", o.Settings.MaxRedirects)
			}
			// Keep our exact headers: never a Go default (User-Agent).
			if _, ok := next.Header["User-Agent"]; !ok {
				next.Header["User-Agent"] = []string{""}
			}
			return nil
		},
	}

	start := o.Now()
	resp, err := client.Do(b.req)
	if err != nil {
		return nil, sendError(ctx, err)
	}
	defer resp.Body.Close()
	counter := &countingReader{r: resp.Body}
	body, encoding, err := decode(counter, resp.Header.Get("Content-Encoding"))
	if err != nil {
		return nil, sendError(ctx, err)
	}
	res.DurationMS = o.Now().Sub(start).Milliseconds()

	res.Status = resp.StatusCode
	res.StatusText = strings.TrimSpace(strings.TrimPrefix(resp.Status, strconv.Itoa(resp.StatusCode)))
	res.Proto = resp.Proto
	res.Method = resp.Request.Method
	res.URL = resp.Request.URL.String()
	res.Headers = headerList(resp.Header)
	res.SentHeaders = headerList(resp.Request.Header)
	host := resp.Request.Host
	if host == "" {
		host = resp.Request.URL.Host
	}
	res.SentHeaders = append([]Header{{Key: "Host", Value: host}}, res.SentHeaders...)
	res.Body = body
	res.Size = int64(len(body))
	res.WireSize = counter.n
	res.Encoding = encoding
	return res, nil
}

// headerList is a header map in a stable order: by name, values in order.
// The empty User-Agent that stops Go from adding its own is not a header.
func headerList(h http.Header) []Header {
	keys := make([]string, 0, len(h))
	for k := range h {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := []Header{}
	for _, k := range keys {
		for _, v := range h[k] {
			if k == "User-Agent" && v == "" {
				continue
			}
			out = append(out, Header{Key: k, Value: v})
		}
	}
	return out
}

// decode reads the whole body and undoes its Content-Encoding (gzip,
// deflate, br, or a list of them, applied in order). An unknown encoding is
// left as it came.
func decode(r io.Reader, encoding string) ([]byte, string, error) {
	raw, err := io.ReadAll(r)
	if err != nil {
		return nil, "", err
	}
	var steps []string
	for _, e := range strings.Split(encoding, ",") {
		if e = strings.ToLower(strings.TrimSpace(e)); e != "" && e != "identity" {
			steps = append(steps, e)
		}
	}
	data := raw
	for i := len(steps) - 1; i >= 0; i-- {
		var rd io.Reader
		switch steps[i] {
		case "gzip", "x-gzip":
			gz, err := gzip.NewReader(bytes.NewReader(data))
			if err != nil {
				return nil, "", apperr.Wrap(CodeDecompress, err)
			}
			rd = gz
		case "deflate":
			rd = deflateReader(data)
		case "br":
			rd = brotli.NewReader(bytes.NewReader(data))
		default:
			return raw, "", nil
		}
		out, err := io.ReadAll(rd)
		if err != nil {
			return nil, "", apperr.Wrap(CodeDecompress, err)
		}
		data = out
	}
	return data, strings.Join(steps, ", "), nil
}

// deflateReader reads "deflate" as servers send it: zlib-wrapped as the RFC
// says, or raw as many do.
func deflateReader(data []byte) io.Reader {
	if len(data) >= 2 && data[0]&0x0f == 8 && (uint16(data[0])<<8|uint16(data[1]))%31 == 0 {
		if zr, err := zlib.NewReader(bytes.NewReader(data)); err == nil {
			return zr
		}
	}
	return flate.NewReader(bytes.NewReader(data))
}

type countingReader struct {
	r io.Reader
	n int64
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += int64(n)
	return n, err
}

// NewTransport builds the transport of a send: the workspace proxy (or the
// system's), its certificate authority and client certificate. Go never
// adds Accept-Encoding nor decompresses: the preview decides.
func NewTransport(n Network) (*http.Transport, error) {
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.DisableCompression = true
	t.Proxy = http.ProxyFromEnvironment
	if p := strings.TrimSpace(n.ProxyURL); p != "" {
		u, err := url.Parse(p)
		if err != nil || u.Host == "" {
			return nil, apperr.New(CodeProxy).With("url", p)
		}
		t.Proxy = http.ProxyURL(u)
	}
	cfg := &tls.Config{InsecureSkipVerify: n.TLSInsecure} //nolint:gosec // the user's choice, per workspace
	if n.CACertPath != "" {
		pem, err := os.ReadFile(n.CACertPath)
		if err != nil {
			return nil, apperr.Wrap(CodeCertificate, err).With("path", n.CACertPath)
		}
		pool, err := x509.SystemCertPool()
		if err != nil || pool == nil {
			pool = x509.NewCertPool()
		}
		if !pool.AppendCertsFromPEM(pem) {
			return nil, apperr.New(CodeCertificate).With("path", n.CACertPath)
		}
		cfg.RootCAs = pool
	}
	if n.ClientCertPath != "" {
		cert, err := tls.LoadX509KeyPair(n.ClientCertPath, n.ClientKeyPath)
		if err != nil {
			return nil, apperr.Wrap(CodeCertificate, err).With("path", n.ClientCertPath)
		}
		cfg.Certificates = []tls.Certificate{cert}
	}
	t.TLSClientConfig = cfg
	return t, nil
}

// sendError turns a failed send into a stable code.
func sendError(ctx context.Context, err error) error {
	var coded *apperr.Error
	switch {
	case errors.As(err, &coded):
		return coded
	case errors.Is(ctx.Err(), context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded):
		return apperr.Wrap(CodeTimeout, err)
	case errors.Is(ctx.Err(), context.Canceled) || errors.Is(err, context.Canceled):
		return apperr.Wrap(CodeCancelled, err)
	}
	var dns *net.DNSError
	if errors.As(err, &dns) {
		return apperr.Wrap(CodeHostNotFound, err).With("host", dns.Name)
	}
	if errors.Is(err, syscall.ECONNREFUSED) {
		return apperr.Wrap(CodeConnRefused, err)
	}
	var certErr *tls.CertificateVerificationError
	var recordErr tls.RecordHeaderError
	var unknownAuth x509.UnknownAuthorityError
	var hostErr x509.HostnameError
	if errors.As(err, &certErr) || errors.As(err, &recordErr) || errors.As(err, &unknownAuth) || errors.As(err, &hostErr) {
		return apperr.Wrap(CodeTLS, err)
	}
	var urlErr *url.Error
	if errors.As(err, &urlErr) && strings.Contains(urlErr.Err.Error(), "proxyconnect") {
		return apperr.Wrap(CodeProxy, err)
	}
	return apperr.Wrap(CodeNetwork, err)
}
