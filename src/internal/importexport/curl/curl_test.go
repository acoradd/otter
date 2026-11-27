package curl

import (
	"errors"
	"reflect"
	"testing"

	"github.com/acoradd/otter/src/internal/apperr"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want Request
	}{
		{
			name: "a plain GET",
			in:   `curl https://api.test/users`,
			want: Request{Method: "GET", URL: "https://api.test/users", BodyType: "none"},
		},
		{
			name: "Chrome, bash: continuations, single quotes, --data-raw JSON",
			in: "curl 'https://api.test/v1/customers?expand=cards' \\\n" +
				"  -H 'accept: application/json' \\\n" +
				"  -H 'content-type: application/json' \\\n" +
				"  -H 'authorization: Bearer abc.def' \\\n" +
				"  --data-raw '{\"name\":\"Ada\",\"note\":\"it''s\"}' \\\n" +
				"  --compressed",
			want: Request{
				Method: "POST", URL: "https://api.test/v1/customers?expand=cards",
				Headers:  []Header{{"accept", "application/json"}, {"authorization", "Bearer abc.def"}},
				BodyType: "json", Body: `{"name":"Ada","note":"its"}`,
			},
		},
		{
			name: "Chrome, cmd: carets everywhere",
			in: "curl ^\"https://api.test/v1/customers^\" ^\r\n" +
				"  -H ^\"content-type: application/json^\" ^\r\n" +
				"  --data-raw ^\"^{^\\^\"name^\\^\":^\\^\"Ada^\\^\"^}^\"",
			want: Request{Method: "POST", URL: "https://api.test/v1/customers", Headers: []Header{}, BodyType: "json", Body: `{"name":"Ada"}`},
		},
		{
			name: "PowerShell: backticks and doubled single quotes",
			in:   "curl.exe -X PUT `\n  -H 'X-Note: it''s fine' `\n  \"https://api.test/a\"",
			want: Request{Method: "PUT", URL: "https://api.test/a", Headers: []Header{{"X-Note", "it's fine"}}, BodyType: "none"},
		},
		{
			name: "double quotes with escapes, attached short options",
			in:   `curl -XDELETE -H"X-Id: \"7\"" "https://api.test/items/7"`,
			want: Request{Method: "DELETE", URL: "https://api.test/items/7", Headers: []Header{{"X-Id", `"7"`}}, BodyType: "none"},
		},
		{
			name: "form data: fields decoded, Otter encodes them again",
			in:   `curl -d 'a=1&b=x%20y' --data-urlencode 'q=a&b' https://api.test/f`,
			want: Request{
				Method: "POST", URL: "https://api.test/f", Headers: []Header{}, BodyType: "form-urlencoded",
				Fields: []Field{{Key: "a", Value: "1"}, {Key: "b", Value: "x y"}, {Key: "q", Value: "a&b"}},
			},
		},
		{
			name: "plain text with -d keeps curl's form Content-Type",
			in:   `curl -d 'hello world' https://api.test/t`,
			want: Request{
				Method: "POST", URL: "https://api.test/t", BodyType: "text", Body: "hello world",
				Headers: []Header{{"Content-Type", "application/x-www-form-urlencoded"}},
			},
		},
		{
			name: "XML by its Content-Type",
			in:   `curl -H 'Content-Type: application/xml' -d '<a/>' https://api.test/x`,
			want: Request{Method: "POST", URL: "https://api.test/x", Headers: []Header{}, BodyType: "xml", Body: "<a/>"},
		},
		{
			name: "a JSON Content-Type with a charset is kept",
			in:   `curl -H 'Content-Type: application/json; charset=utf-8' -d '{}' https://api.test/j`,
			want: Request{Method: "POST", URL: "https://api.test/j", Headers: []Header{{"Content-Type", "application/json; charset=utf-8"}}, BodyType: "json", Body: "{}"},
		},
		{
			name: "--data-binary @file is a file body",
			in:   `curl --data-binary @/tmp/export.bin -H 'Content-Type: application/octet-stream' https://api.test/up`,
			want: Request{Method: "POST", URL: "https://api.test/up", Headers: []Header{}, BodyType: "binary", BodyFile: "/tmp/export.bin"},
		},
		{
			name: "multipart: text, file with a type, form-string",
			in:   `curl -F name=Ada -F 'avatar=@"/home/ada/me.png";type=image/png' --form-string 'raw=@kept' https://api.test/m`,
			want: Request{
				Method: "POST", URL: "https://api.test/m", Headers: []Header{}, BodyType: "multipart",
				Fields: []Field{
					{Key: "raw", Value: "@kept"},
					{Key: "name", Value: "Ada"},
					{Key: "avatar", Value: "/home/ada/me.png", File: true, ContentType: "image/png"},
				},
			},
		},
		{
			name: "-u, -b, -A, -e, -L, --max-time, -G",
			in:   `curl -u ada:pw -b 'sid=1; theme=dark' -A 'Bot/1' -e https://from.test -L --max-time 2.5 -G -d q=x -d page=2 https://api.test/s?a=1`,
			want: Request{
				Method: "GET", URL: "https://api.test/s?a=1&q=x&page=2", BodyType: "none",
				Headers: []Header{
					{"Authorization", "Basic YWRhOnB3"}, {"Cookie", "sid=1; theme=dark"}, {"User-Agent", "Bot/1"}, {"Referer", "https://from.test"},
				},
				FollowRedirects: true, TimeoutMS: 2500,
			},
		},
		{
			name: "--json, -I, a URL without scheme",
			in:   `curl -I api.test/health`,
			want: Request{Method: "HEAD", URL: "http://api.test/health", BodyType: "none"},
		},
		{
			name: "--json sets JSON and Accept",
			in:   `curl --json '{"a":1}' https://api.test/j`,
			want: Request{Method: "POST", URL: "https://api.test/j", Headers: []Header{{"Accept", "application/json"}}, BodyType: "json", Body: `{"a":1}`},
		},
		{
			name: "options without equivalent are listed; output ones are not",
			in:   `curl -sSk --cert c.pem --proxy http://p:1 -o out.json -b cookies.txt --url https://api.test/`,
			want: Request{Method: "GET", URL: "https://api.test/", BodyType: "none", Ignored: []string{"-k", "--cert", "--proxy", "-b cookies.txt"}},
		},
		{
			name: "an empty header with ;",
			in:   `curl -H 'X-Empty;' https://api.test/`,
			want: Request{Method: "GET", URL: "https://api.test/", Headers: []Header{{"X-Empty", ""}}, BodyType: "none"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Parse(tt.in)
			if err != nil {
				t.Fatal(err)
			}
			norm := func(r *Request) {
				if r.Headers == nil {
					r.Headers = []Header{}
				}
				if r.Fields == nil {
					r.Fields = []Field{}
				}
				if r.Ignored == nil {
					r.Ignored = []string{}
				}
			}
			norm(&got)
			norm(&tt.want)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got  %+v\nwant %+v", got, tt.want)
			}
		})
	}
}

func TestParseRefuses(t *testing.T) {
	for in, reason := range map[string]string{
		"https://api.test/users":         "not_curl",
		"wget https://api.test":          "not_curl",
		"":                               "not_curl",
		"curl -H 'X: 1'":                 "no_url",
		"curl 'https://api.test/x":       "quote",
		`curl -H "unterminated https://`: "quote",
	} {
		_, err := Parse(in)
		var e *apperr.Error
		if !errors.As(err, &e) || e.Code != CodeInvalid || e.Params["reason"] != reason {
			t.Errorf("%q: %v, want %s (%s)", in, err, CodeInvalid, reason)
		}
	}
}
