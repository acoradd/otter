// Package curl reads a curl command line into a request: pasted in the URL
// bar or imported (docs/specs/scope-v1.md, section 7).
package curl

import (
	"encoding/base64"
	"encoding/json"
	"net/url"
	"strconv"
	"strings"

	"github.com/acoradd/otter/src/internal/apperr"
)

// CodeInvalid is text that is not a curl command Otter can read (param
// reason: "quote", "not_curl", "no_url").
const CodeInvalid apperr.Code = "CURL_INVALID"

// Header is a header of the command, in order.
type Header struct {
	Key   string
	Value string
}

// Field is a field of a form URL-encoded or multipart body.
type Field struct {
	Key         string
	Value       string
	File        bool
	ContentType string
}

// Request is what the command asks for, in Otter's terms.
type Request struct {
	Method   string
	URL      string
	Headers  []Header
	BodyType string // none, json, xml, text, form-urlencoded, multipart, binary
	Body     string
	BodyFile string
	Fields   []Field
	// FollowRedirects: -L. curl does not follow without it.
	FollowRedirects bool
	// TimeoutMS from --max-time, 0 when not given.
	TimeoutMS int
	// Ignored are the options Otter has no equivalent for (-k, --cert…).
	Ignored []string
}

// Options taking a value, short and long.
var shortWithValue = "AbcdDeEFHmorTuUwxXKCyYzQ"

var longWithValue = map[string]bool{
	"request": true, "header": true, "data": true, "data-raw": true, "data-binary": true, "data-ascii": true,
	"data-urlencode": true, "form": true, "form-string": true, "user": true, "cookie": true, "user-agent": true,
	"referer": true, "max-time": true, "url": true, "output": true, "connect-timeout": true, "cert": true,
	"key": true, "cacert": true, "capath": true, "proxy": true, "proxy-user": true, "retry": true, "json": true,
	"cookie-jar": true, "write-out": true, "range": true, "upload-file": true, "dump-header": true,
	"resolve": true, "interface": true, "limit-rate": true, "max-redirs": true, "oauth2-bearer": true,
	"cert-type": true, "key-type": true, "pass": true, "ciphers": true, "config": true, "trace": true,
	"trace-ascii": true, "unix-socket": true, "retry-delay": true, "retry-max-time": true, "time-cond": true,
	"aws-sigv4": true, "header-file": true, "variable": true, "expand-url": true,
}

// Options that only shape curl's own output: dropped without a word.
var quiet = map[string]bool{
	"s": true, "silent": true, "S": true, "show-error": true, "v": true, "verbose": true, "i": true,
	"include": true, "o": true, "output": true, "O": true, "remote-name": true, "w": true, "write-out": true,
	"#": true, "progress-bar": true, "no-progress-meter": true, "f": true, "fail": true, "fail-with-body": true,
	"compressed": true, "D": true, "dump-header": true, "trace": true, "trace-ascii": true,
}

// shorts maps short options to their long names.
var shorts = map[string]string{
	"X": "request", "H": "header", "d": "data", "F": "form", "u": "user", "b": "cookie", "A": "user-agent",
	"e": "referer", "L": "location", "m": "max-time", "G": "get", "I": "head", "k": "insecure", "E": "cert",
	"x": "proxy", "T": "upload-file", "c": "cookie-jar", "r": "range", "U": "proxy-user", "K": "config",
	"o": "output", "w": "write-out", "D": "dump-header",
}

type dataPart struct {
	kind  string // data, raw, binary, urlencode
	value string
}

// Parse reads a curl command.
func Parse(command string) (Request, error) {
	words, err := tokenize(strings.TrimSpace(command))
	if err != nil {
		return Request{}, err
	}
	if len(words) == 0 || !isCurl(words[0]) {
		return Request{}, apperr.New(CodeInvalid).With("reason", "not_curl")
	}

	r := Request{Headers: []Header{}, Fields: []Field{}, Ignored: []string{}}
	var method string
	var data []dataPart
	var forms []string
	var get, head, jsonFlag bool

	args := words[1:]
	for i := 0; i < len(args); i++ {
		a := args[i]
		if !strings.HasPrefix(a, "-") || a == "-" {
			if r.URL == "" {
				r.URL = a
			}
			continue
		}
		// One option, or several short ones together (-sSL, -XPOST).
		var opts [][3]string // name, value, as typed
		if strings.HasPrefix(a, "--") {
			name, value, hasValue := strings.Cut(a[2:], "=")
			if longWithValue[name] && !hasValue {
				if i+1 < len(args) {
					i++
					value = args[i]
				}
			}
			opts = append(opts, [3]string{name, value, "--" + name})
		} else {
			for j := 1; j < len(a); j++ {
				c := string(a[j])
				name := shorts[c]
				if name == "" {
					name = c
				}
				if strings.Contains(shortWithValue, c) {
					value := a[j+1:]
					if value == "" && i+1 < len(args) {
						i++
						value = args[i]
					}
					opts = append(opts, [3]string{name, value, "-" + c})
					break
				}
				opts = append(opts, [3]string{name, "", "-" + c})
			}
		}

		for _, o := range opts {
			name, value, typed := o[0], o[1], o[2]
			switch name {
			case "request":
				method = strings.ToUpper(value)
			case "header":
				if k, v, ok := strings.Cut(value, ":"); ok {
					r.Headers = append(r.Headers, Header{Key: strings.TrimSpace(k), Value: strings.TrimSpace(v)})
				} else if k, ok := strings.CutSuffix(value, ";"); ok {
					r.Headers = append(r.Headers, Header{Key: strings.TrimSpace(k)}) // "X-Empty;": an empty header
				}
			case "data", "data-ascii":
				data = append(data, dataPart{"data", value})
			case "data-raw":
				data = append(data, dataPart{"raw", value})
			case "data-binary":
				data = append(data, dataPart{"binary", value})
			case "data-urlencode":
				data = append(data, dataPart{"urlencode", value})
			case "json":
				data = append(data, dataPart{"raw", value})
				jsonFlag = true
			case "form":
				forms = append(forms, value)
			case "form-string":
				k, v, _ := strings.Cut(value, "=")
				r.Fields = append(r.Fields, Field{Key: k, Value: v})
			case "user":
				user := value
				if !strings.Contains(user, ":") {
					user += ":"
				}
				r.Headers = append(r.Headers, Header{Key: "Authorization", Value: "Basic " + base64.StdEncoding.EncodeToString([]byte(user))})
			case "oauth2-bearer":
				r.Headers = append(r.Headers, Header{Key: "Authorization", Value: "Bearer " + value})
			case "cookie":
				if strings.Contains(value, "=") {
					r.Headers = append(r.Headers, Header{Key: "Cookie", Value: value})
				} else {
					r.Ignored = append(r.Ignored, typed+" "+value) // a cookie file
				}
			case "user-agent":
				r.Headers = append(r.Headers, Header{Key: "User-Agent", Value: value})
			case "referer":
				r.Headers = append(r.Headers, Header{Key: "Referer", Value: value})
			case "location":
				r.FollowRedirects = true
			case "max-time":
				if s, err := strconv.ParseFloat(value, 64); err == nil && s > 0 {
					r.TimeoutMS = int(s * 1000)
				}
			case "get":
				get = true
			case "head":
				head = true
			case "url":
				r.URL = value
			default:
				if !quiet[name] {
					r.Ignored = append(r.Ignored, typed)
				}
			}
		}
	}
	if r.URL == "" {
		return Request{}, apperr.New(CodeInvalid).With("reason", "no_url")
	}
	if !strings.Contains(r.URL, "://") {
		r.URL = "http://" + r.URL // as curl does
	}

	// The body.
	r.BodyType = "none"
	switch {
	case len(forms) > 0 || len(r.Fields) > 0:
		r.BodyType = "multipart"
		for _, f := range forms {
			r.Fields = append(r.Fields, formField(f))
		}
	case get && len(data) > 0:
		// -G: the data goes in the query.
		var q []string
		for _, d := range data {
			q = append(q, d.value)
		}
		sep := "?"
		if strings.Contains(r.URL, "?") {
			sep = "&"
		}
		r.URL += sep + strings.Join(q, "&")
	case len(data) > 0:
		r.body(data, jsonFlag)
	}

	switch {
	case method != "":
		r.Method = method
	case head:
		r.Method = "HEAD"
	case r.BodyType != "none" && !get:
		r.Method = "POST"
	default:
		r.Method = "GET"
	}
	if jsonFlag && headerIndex(r.Headers, "Accept") < 0 {
		r.Headers = append(r.Headers, Header{Key: "Accept", Value: "application/json"})
	}
	return r, nil
}

func isCurl(word string) bool {
	w := strings.ToLower(word)
	return w == "curl" || w == "curl.exe" || strings.HasSuffix(w, "/curl") || strings.HasSuffix(w, `\curl.exe`)
}

// formField reads -F name=value, name=@file;type=…, name=<file.
func formField(spec string) Field {
	name, value, _ := strings.Cut(spec, "=")
	f := Field{Key: name}
	if strings.HasPrefix(value, "@") || strings.HasPrefix(value, "<") {
		f.File = true
		value = value[1:]
	}
	parts := strings.Split(value, ";")
	f.Value = strings.Trim(parts[0], `"`)
	for _, p := range parts[1:] {
		if k, v, ok := strings.Cut(strings.TrimSpace(p), "="); ok && strings.EqualFold(k, "type") {
			f.ContentType = v
		}
	}
	return f
}

// body turns -d, --data-raw, --data-binary and --data-urlencode into an
// Otter body: a file, form fields, or text typed by its Content-Type or its
// content.
func (r *Request) body(data []dataPart, jsonFlag bool) {
	ct := ""
	if i := headerIndex(r.Headers, "Content-Type"); i >= 0 {
		ct = strings.ToLower(r.Headers[i].Value)
	}
	if jsonFlag && ct == "" {
		ct = "application/json"
	}
	// A single @file: the file is the body.
	if len(data) == 1 && (data[0].kind == "data" || data[0].kind == "binary") && strings.HasPrefix(data[0].value, "@") {
		r.BodyType, r.BodyFile = "binary", data[0].value[1:]
		r.dropContentType("")
		return
	}

	var parts []string
	urlencoded := false
	for _, d := range data {
		if d.kind == "urlencode" {
			urlencoded = true
			parts = append(parts, encodeData(d.value))
		} else {
			parts = append(parts, d.value)
		}
	}
	body := strings.Join(parts, "&")

	isJSON := json.Valid([]byte(body)) && (strings.HasPrefix(strings.TrimSpace(body), "{") || strings.HasPrefix(strings.TrimSpace(body), "["))
	switch {
	case strings.Contains(ct, "json") || (ct == "" && isJSON):
		r.BodyType, r.Body = "json", body
		r.dropContentType("application/json")
	case strings.Contains(ct, "xml"):
		r.BodyType, r.Body = "xml", body
		r.dropContentType("application/xml")
	case (ct == "" || strings.Contains(ct, "x-www-form-urlencoded")) && (urlencoded || looksLikeForm(body)):
		r.BodyType = "form-urlencoded"
		for _, pair := range strings.Split(body, "&") {
			k, v, _ := strings.Cut(pair, "=")
			k, _ = url.QueryUnescape(k)
			v, _ = url.QueryUnescape(v)
			r.Fields = append(r.Fields, Field{Key: k, Value: v})
		}
		r.dropContentType("application/x-www-form-urlencoded")
	default:
		r.BodyType, r.Body = "text", body
		if ct == "" {
			// curl sends -d as a form: keep its Content-Type.
			r.Headers = append(r.Headers, Header{Key: "Content-Type", Value: "application/x-www-form-urlencoded"})
		} else {
			r.dropContentType("text/plain")
		}
	}
}

// dropContentType removes a Content-Type header equal to the automatic one
// of the body type (or any, with ""): Otter adds it.
func (r *Request) dropContentType(auto string) {
	i := headerIndex(r.Headers, "Content-Type")
	if i < 0 {
		return
	}
	if auto == "" || strings.EqualFold(strings.TrimSpace(r.Headers[i].Value), auto) {
		r.Headers = append(r.Headers[:i], r.Headers[i+1:]...)
	}
}

func headerIndex(headers []Header, key string) int {
	for i, h := range headers {
		if strings.EqualFold(h.Key, key) {
			return i
		}
	}
	return -1
}

// encodeData encodes --data-urlencode content as curl does: name=value
// encodes the value, =value and value encode it all.
func encodeData(s string) string {
	if name, value, ok := strings.Cut(s, "="); ok {
		if name == "" {
			return url.QueryEscape(value)
		}
		return name + "=" + url.QueryEscape(value)
	}
	return url.QueryEscape(s)
}

// looksLikeForm: name=value pairs joined by &, nothing else.
func looksLikeForm(body string) bool {
	if body == "" || strings.ContainsAny(body, " \n\t{}[]\"") {
		return false
	}
	for _, pair := range strings.Split(body, "&") {
		k, _, ok := strings.Cut(pair, "=")
		if !ok || k == "" {
			return false
		}
	}
	return true
}
