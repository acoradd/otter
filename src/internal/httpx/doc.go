// Package httpx sends a request built from a resolve.Preview: client per
// send (proxy, TLS, cookies), redirects, URL encoding, streamed bodies
// (multipart and files), decompression, cancellation.
//
// HTTP behaviour is tested against httptest.Server (docs/specs/scope-v1.md,
// section 7).
package httpx
