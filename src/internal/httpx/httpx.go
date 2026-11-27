// Package httpx sends a request built from a resolve.Preview, and nothing
// else: what the user saw is what leaves. It builds a client per send
// (proxy, TLS, cookies), follows redirects up to a limit and records them,
// encodes the URL or sends it as written, streams multipart and file bodies,
// and decompresses gzip, deflate and brotli itself.
//
// HTTP behaviour is tested against httptest.Server (docs/specs/scope-v1.md,
// section 7).
package httpx

import (
	"github.com/acoradd/otter/src/internal/apperr"
)

// Error codes of a send. The request may still have left.
const (
	CodeCancelled        apperr.Code = "SEND_CANCELLED"
	CodeTimeout          apperr.Code = "SEND_TIMEOUT"
	CodeTooManyRedirects apperr.Code = "TOO_MANY_REDIRECTS"
	CodeHostNotFound     apperr.Code = "HOST_NOT_FOUND"
	CodeConnRefused      apperr.Code = "CONNECTION_REFUSED"
	CodeTLS              apperr.Code = "TLS_ERROR"
	CodeBadURL           apperr.Code = "INVALID_URL"
	CodeFileNotFound     apperr.Code = "FILE_NOT_FOUND" // path
	CodeProxy            apperr.Code = "PROXY_ERROR"
	CodeNetwork          apperr.Code = "NETWORK_ERROR"
	CodeDecompress       apperr.Code = "DECOMPRESS_ERROR"
	CodeCertificate      apperr.Code = "CERTIFICATE_FILE_ERROR" // path
)

// Settings are the request's own sending settings.
type Settings struct {
	EncodeURL       bool
	FollowRedirects bool
	MaxRedirects    int
	TimeoutMS       int
}

// Network are the workspace's network settings.
type Network struct {
	// ProxyURL is the proxy to use; empty uses the system's (environment).
	ProxyURL    string
	TLSInsecure bool
	// CACertPath is an extra certificate authority (PEM), trusted with the
	// system's.
	CACertPath string
	// ClientCertPath and ClientKeyPath are a client certificate (PEM).
	ClientCertPath string
	ClientKeyPath  string
}

// Header is a header as received, in order.
type Header struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// Redirect is a response that sent the client elsewhere.
type Redirect struct {
	Status   int    `json:"status"`
	URL      string `json:"url"`
	Location string `json:"location"`
}

// SentPart describes what a body part was made of, for the history: files
// are recorded by name and size, never their content.
type SentPart struct {
	Name string `json:"name"`
	// File is the path of a file part; Size its size in bytes.
	File string `json:"file,omitempty"`
	Size int64  `json:"size,omitempty"`
}

// Response is what came back.
type Response struct {
	Status     int    `json:"status"`
	StatusText string `json:"statusText"`
	Proto      string `json:"proto"`
	// Method is the method of the last request: a 301, 302 or 303 turns a
	// POST into a GET, as browsers do.
	Method string `json:"method"`
	// URL is the final URL, after redirects.
	URL       string     `json:"url"`
	Headers   []Header   `json:"headers"`
	Redirects []Redirect `json:"redirects"`
	// Body is decompressed. Size is its length; WireSize what travelled
	// (compressed, when it was).
	Body     []byte `json:"-"`
	Size     int64  `json:"size"`
	WireSize int64  `json:"wireSize"`
	// Encoding is the Content-Encoding that was undone, "" for none.
	Encoding   string `json:"encoding"`
	DurationMS int64  `json:"durationMs"`
	// SentHeaders are the request headers as they left (the last hop).
	SentHeaders []Header `json:"sentHeaders"`
	// SentParts describes a multipart or file body.
	SentParts []SentPart `json:"sentParts"`
}
