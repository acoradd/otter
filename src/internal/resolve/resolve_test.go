package resolve

import (
	"reflect"
	"strings"
	"testing"
)

// --- Builders ------------------------------------------------------------

func v(key, value string) Var    { return Var{Key: key, Value: value, Enabled: true} }
func off(x Var) Var              { x.Enabled = false; return x }
func secret(x Var) Var           { x.Secret = true; return x }
func h(key, value string) Header { return Header{Key: key, Value: value, Enabled: true} }
func ignore(key string) Header   { return Header{Key: key, Ignore: true, Enabled: true} }
func hoff(x Header) Header       { x.Enabled = false; return x }
func folder(id int64, path ...string) Folder {
	return Folder{ID: id, Name: path[len(path)-1], Path: path}
}

// api > users > admin, env "dev", a GET request.
func base() Input {
	return Input{
		Folders: []Folder{folder(1, "api"), folder(2, "api", "users"), folder(3, "api", "users", "admin")},
		Env:     &Env{ID: 9, Name: "dev"},
		Request: &Request{Method: "get", URL: "https://api.test/users", BodyType: "none"},
		OS:      "linux",
	}
}

func varByKey(p Preview, key string) *ResolvedVar {
	for i := range p.Vars {
		if p.Vars[i].Key == key {
			return &p.Vars[i]
		}
	}
	return nil
}

func headerByKey(list []ResolvedHeader, key string) *ResolvedHeader {
	for i := range list {
		if strings.EqualFold(list[i].Key, key) {
			return &list[i]
		}
	}
	return nil
}

func codes(p Preview) []string {
	var out []string
	for _, w := range p.Warnings {
		s := w.Code
		for _, k := range []string{"name", "url"} {
			if w.Params[k] != "" {
				s += ":" + w.Params[k]
			}
		}
		out = append(out, s)
	}
	return out
}

// --- Variables -----------------------------------------------------------

func TestVariablePriorities(t *testing.T) {
	tests := []struct {
		name   string
		setup  func(in *Input)
		value  string
		source SourceKind
		depth  int
		hidden []string // shadowed values, closest first
	}{
		{"environment only", func(in *Input) { in.Env.Vars = []Var{v("x", "env")} }, "env", SourceEnv, 0, nil},
		{"folder beats environment", func(in *Input) {
			in.Env.Vars = []Var{v("x", "env")}
			in.Folders[0].Vars = []Var{v("x", "api")}
		}, "api", SourceFolder, 2, []string{"env"}},
		{"closest folder wins", func(in *Input) {
			in.Folders[0].Vars = []Var{v("x", "api")}
			in.Folders[2].Vars = []Var{v("x", "admin")}
			in.Folders[1].Vars = []Var{v("x", "users")}
		}, "admin", SourceFolder, 0, []string{"users", "api"}},
		{"request beats folders", func(in *Input) {
			in.Folders[2].Vars = []Var{v("x", "admin")}
			in.Request.Vars = []Var{v("x", "req")}
		}, "req", SourceRequest, 0, []string{"admin"}},
		{"runtime beats everything", func(in *Input) {
			in.Env.Vars = []Var{v("x", "env")}
			in.Request.Vars = []Var{v("x", "req")}
			in.Runtime = []Var{v("x", "rt")}
		}, "rt", SourceRuntime, 0, []string{"req", "env"}},
		{"disabled rows do not count", func(in *Input) {
			in.Env.Vars = []Var{v("x", "env")}
			in.Request.Vars = []Var{off(v("x", "req"))}
		}, "env", SourceEnv, 0, nil},
		{"last row of a layer wins", func(in *Input) {
			in.Request.Vars = []Var{v("x", "first"), v("x", "second")}
		}, "second", SourceRequest, 0, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := base()
			tt.setup(&in)
			p := Resolve(in)
			got := varByKey(p, "x")
			if got == nil || got.Value != tt.value || got.Source == nil || got.Source.Kind != tt.source || got.Source.Depth != tt.depth {
				t.Fatalf("x = %+v, want %q from %s (depth %d)", got, tt.value, tt.source, tt.depth)
			}
			var hidden []string
			for _, c := range got.Shadowed {
				hidden = append(hidden, c.Value)
			}
			if !reflect.DeepEqual(hidden, tt.hidden) {
				t.Errorf("shadowed = %v, want %v", hidden, tt.hidden)
			}
		})
	}
}

func TestInterpolation(t *testing.T) {
	tests := []struct {
		name     string
		vars     []Var
		url      string
		want     string
		missing  []string
		warnings []string
	}{
		{"plain", []Var{v("host", "api.test")}, "https://{{host}}/x", "https://api.test/x", []string{}, nil},
		{"spaces tolerated", []Var{v("host", "api.test")}, "https://{{ host }}/x", "https://api.test/x", []string{}, nil},
		{"names with dots and dashes", []Var{v("a.b-c_1", "ok")}, "https://h/{{a.b-c_1}}", "https://h/ok", []string{}, nil},
		{"recursive", []Var{v("base", "https://{{host}}/v{{ver}}"), v("host", "api.test"), v("ver", "2")}, "{{base}}/x", "https://api.test/v2/x", []string{}, nil},
		{"missing stays as written", nil, "https://{{host}}/{{ id }}", "https://{{host}}/{{ id }}", []string{"host", "id"},
			[]string{"UNRESOLVED_VAR:host", "UNRESOLVED_VAR:id"}},
		{"missing deep inside a value", []Var{v("base", "https://{{host}}")}, "{{base}}/x", "https://{{host}}/x", []string{"host"},
			[]string{"UNRESOLVED_VAR:host"}},
		{"cycle stays as written", []Var{v("a", "{{b}}"), v("b", "{{a}}")}, "https://h/{{a}}", "https://h/{{a}}", []string{},
			[]string{"VAR_CYCLE:a"}},
		{"self reference", []Var{v("a", "x{{a}}")}, "https://h/{{a}}", "https://h/x{{a}}", []string{}, []string{"VAR_CYCLE:a"}},
		{"not a placeholder", nil, "https://h/{x}/{{}}/{{a b}}", "https://h/{x}/{{}}/{{a b}}", []string{}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := base()
			in.Env.Vars = tt.vars
			in.Request.URL = tt.url
			p := Resolve(in)
			if p.URL != tt.want {
				t.Errorf("URL = %q, want %q", p.URL, tt.want)
			}
			if !reflect.DeepEqual(p.Missing, tt.missing) {
				t.Errorf("missing = %v, want %v", p.Missing, tt.missing)
			}
			if got := codes(p); !reflect.DeepEqual(got, tt.warnings) {
				t.Errorf("warnings = %v, want %v", got, tt.warnings)
			}
		})
	}
}

func TestMaxDepth(t *testing.T) {
	var vars []Var
	for i := 0; i < MaxDepth+2; i++ {
		vars = append(vars, v("v"+itoa(i), "{{v"+itoa(i+1)+"}}"))
	}
	vars = append(vars, v("v"+itoa(MaxDepth+2), "end"))
	in := base()
	in.Env.Vars = vars
	in.Request.URL = "https://h/{{v0}}"
	p := Resolve(in)
	if !strings.Contains(p.URL, "{{") {
		t.Fatalf("a chain deeper than %d should stop: %q", MaxDepth, p.URL)
	}
	if len(p.Warnings) == 0 || p.Warnings[0].Code != WarnVarCycle {
		t.Fatalf("warnings = %v, want a VAR_CYCLE", p.Warnings)
	}

	in.Env.Vars = vars[MaxDepth-1:] // a chain of exactly MaxDepth levels
	in.Request.URL = "https://h/{{v" + itoa(MaxDepth-1) + "}}"
	if p := Resolve(in); p.URL != "https://h/end" {
		t.Fatalf("a %d-level chain should resolve: %q %v", MaxDepth, p.URL, p.Warnings)
	}
}

func TestEverywhereInterpolated(t *testing.T) {
	in := base()
	in.Env.Vars = []Var{v("name", "Ada"), v("hdr", "X-User"), v("dir", "/tmp")}
	in.Request.Headers = []Header{h("{{hdr}}", "{{name}}")}
	in.Request.BodyType = "multipart"
	in.Request.Body = "{{name}} is ignored for multipart"
	in.Request.Fields = []Field{
		{Key: "who", Value: "{{name}}", Enabled: true},
		{Key: "avatar", Value: "{{dir}}/a.png", File: true, ContentType: " image/png ", Enabled: true},
		{Key: "off", Value: "x", Enabled: false},
	}
	p := Resolve(in)

	if got := headerByKey(p.Headers, "X-User"); got == nil || got.Value != "Ada" || got.RawValue != "{{name}}" {
		t.Errorf("header = %+v", got)
	}
	want := []ResolvedField{{Key: "who", Value: "Ada"}, {Key: "avatar", Value: "/tmp/a.png", File: true, ContentType: "image/png"}}
	if !reflect.DeepEqual(p.BodyFields, want) {
		t.Errorf("fields = %+v, want %+v", p.BodyFields, want)
	}
	if p.Body != "" {
		t.Errorf("a multipart request has no text body: %q", p.Body)
	}
	if got := varByKey(p, "name").UsedIn; !reflect.DeepEqual(got, []string{"field:who", "header:X-User"}) {
		t.Errorf("name used in %v", got)
	}

	in.Request.BodyType = "binary"
	in.Request.BodyFile = "{{dir}}/export.bin"
	if p := Resolve(in); p.BodyFile != "/tmp/export.bin" || len(p.BodyFields) != 0 {
		t.Errorf("binary body = %q, fields %v", p.BodyFile, p.BodyFields)
	}

	in.Request.BodyType = "json"
	in.Request.Body = `{"who":"{{name}}"}`
	if p := Resolve(in); p.Body != `{"who":"Ada"}` {
		t.Errorf("json body = %q", p.Body)
	}
}

func TestSecrets(t *testing.T) {
	in := base()
	in.Env.Vars = []Var{secret(v("token", "s3cr3t")), v("auth", "Bearer {{token}}"), v("plain", "x")}
	in.Request.Headers = []Header{h("Authorization", "{{auth}}"), h("X-Plain", "{{plain}}")}
	p := Resolve(in)
	if !varByKey(p, "token").Secret || !varByKey(p, "auth").Secret || varByKey(p, "plain").Secret {
		t.Errorf("secret flags: token %v auth %v plain %v", varByKey(p, "token").Secret, varByKey(p, "auth").Secret, varByKey(p, "plain").Secret)
	}
	if !headerByKey(p.Headers, "Authorization").Secret || headerByKey(p.Headers, "X-Plain").Secret {
		t.Error("a header holding a secret is secret, and only that one")
	}
	// A request variable redefining it is no secret.
	in.Request.Vars = []Var{v("token", "public")}
	if Resolve(in).Vars[0].Secret {
		t.Error("the request's own value is not a secret")
	}
}

// --- Headers -------------------------------------------------------------

func TestHeaderLayers(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(in *Input)
		key       string
		value     string
		source    SourceKind
		ignored   bool
		ignoredBy SourceKind
		shadowed  []string
	}{
		{"set by a folder", func(in *Input) { in.Folders[0].Headers = []Header{h("X-Team", "core")} },
			"X-Team", "core", SourceFolder, false, "", nil},
		{"closest folder replaces", func(in *Input) {
			in.Folders[0].Headers = []Header{h("X-Team", "core")}
			in.Folders[2].Headers = []Header{h("x-team", "admin")}
		}, "x-team", "admin", SourceFolder, false, "", []string{"core"}},
		{"request replaces", func(in *Input) {
			in.Folders[1].Headers = []Header{h("X-Team", "users")}
			in.Request.Headers = []Header{h("X-TEAM", "me")}
		}, "X-TEAM", "me", SourceRequest, false, "", []string{"users"}},
		{"a folder ignores for its subtree", func(in *Input) {
			in.Folders[0].Headers = []Header{h("X-Team", "core")}
			in.Folders[1].Headers = []Header{ignore("x-team")}
		}, "X-Team", "core", SourceFolder, true, SourceFolder, nil},
		{"redefined after an ignore", func(in *Input) {
			in.Folders[0].Headers = []Header{h("X-Team", "core")}
			in.Folders[1].Headers = []Header{ignore("X-Team")}
			in.Request.Headers = []Header{h("X-Team", "back")}
		}, "X-Team", "back", SourceRequest, false, "", []string{"core"}},
		{"ignore of nothing does nothing", func(in *Input) { in.Request.Headers = []Header{ignore("X-None")} },
			"X-None", "", "", false, "", nil},
		{"disabled rows do not count", func(in *Input) {
			in.Folders[0].Headers = []Header{h("X-Team", "core")}
			in.Request.Headers = []Header{hoff(h("X-Team", "me")), hoff(ignore("X-Team"))}
		}, "X-Team", "core", SourceFolder, false, "", nil},
		{"request ignores an automatic header", func(in *Input) { in.Request.Headers = []Header{ignore("user-agent")} },
			"User-Agent", UserAgent("linux"), SourceAuto, true, SourceRequest, nil},
		{"a folder's header beats the automatic one", func(in *Input) { in.Folders[0].Headers = []Header{h("Accept", "application/json")} },
			"Accept", "application/json", SourceFolder, false, "", nil},
		{"request replaces an automatic header", func(in *Input) { in.Request.Headers = []Header{h("Accept-Encoding", "identity")} },
			"Accept-Encoding", "identity", SourceRequest, false, "", []string{"gzip, deflate, br"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := base()
			tt.setup(&in)
			p := Resolve(in)
			got := headerByKey(p.Headers, tt.key)
			if tt.source == "" {
				if got != nil {
					t.Fatalf("%s should not exist: %+v", tt.key, got)
				}
				return
			}
			if got == nil {
				t.Fatalf("%s missing from %+v", tt.key, p.Headers)
			}
			if got.Key != tt.key || got.Value != tt.value || got.Source.Kind != tt.source || got.Ignored != tt.ignored {
				t.Fatalf("%s = %+v", tt.key, got)
			}
			if tt.ignored && (got.IgnoredBy == nil || got.IgnoredBy.Kind != tt.ignoredBy) {
				t.Errorf("ignored by %+v, want %s", got.IgnoredBy, tt.ignoredBy)
			}
			var hidden []string
			for _, c := range got.Shadowed {
				hidden = append(hidden, c.Value)
			}
			if !reflect.DeepEqual(hidden, tt.shadowed) {
				t.Errorf("shadowed = %v, want %v", hidden, tt.shadowed)
			}
		})
	}
}

func TestAutomaticHeaders(t *testing.T) {
	tests := []struct {
		bodyType, file, contentType string
		computed                    bool
	}{
		{"none", "", "", false},
		{"json", "", "application/json", false},
		{"xml", "", "application/xml", false},
		{"text", "", "text/plain", false},
		{"form-urlencoded", "", "application/x-www-form-urlencoded", false},
		{"multipart", "", "multipart/form-data", true},
		{"binary", "/data/photo.PNG", "image/png", false},
		{"binary", `C:\data\report.pdf`, "application/pdf", false},
		{"binary", "/data/blob", "application/octet-stream", false},
	}
	for _, tt := range tests {
		t.Run(tt.bodyType+tt.file, func(t *testing.T) {
			in := base()
			in.Request.BodyType, in.Request.BodyFile = tt.bodyType, tt.file
			p := Resolve(in)
			ct := headerByKey(p.Headers, "Content-Type")
			if tt.contentType == "" {
				if ct != nil {
					t.Fatalf("no body, no Content-Type: %+v", ct)
				}
			} else if ct == nil || ct.Value != tt.contentType || ct.Computed != tt.computed || ct.Source.Kind != SourceAuto {
				t.Fatalf("Content-Type = %+v, want %q computed=%v", ct, tt.contentType, tt.computed)
			}
			for key, want := range map[string]string{"Accept": "*/*", "Accept-Encoding": "gzip, deflate, br", "User-Agent": UserAgent("linux")} {
				if got := headerByKey(p.Headers, key); got == nil || got.Value != want || got.Source.Kind != SourceAuto {
					t.Errorf("%s = %+v", key, got)
				}
			}
		})
	}
}

func TestUserAgent(t *testing.T) {
	major := itoa(ChromeMajorVersion)
	for os, platform := range map[string]string{
		"windows": "Windows NT 10.0; Win64; x64",
		"darwin":  "Macintosh; Intel Mac OS X 10_15_7",
		"linux":   "X11; Linux x86_64",
		"freebsd": "X11; Linux x86_64",
	} {
		want := "Mozilla/5.0 (" + platform + ") AppleWebKit/537.36 (KHTML, like Gecko) Chrome/" + major + ".0.0.0 Safari/537.36"
		if got := UserAgent(os); got != want {
			t.Errorf("%s: %q", os, got)
		}
	}
}

func TestComputedHeaders(t *testing.T) {
	in := base()
	in.Request.URL = "https://api.test:8443/users?q=1"
	in.Request.BodyType = "json"
	in.Request.Body = `{"é":1}`
	p := Resolve(in)
	if got := headerByKey(p.Headers, "Host"); got == nil || got.Value != "api.test:8443" || !got.ReadOnly {
		t.Errorf("Host = %+v", got)
	}
	if got := headerByKey(p.Headers, "Content-Length"); got == nil || got.Value != "8" || !got.ReadOnly || got.Computed {
		t.Errorf("Content-Length = %+v: bytes, not characters", got)
	}

	in.Request.BodyType = "form-urlencoded"
	in.Request.Fields = []Field{{Key: "q", Value: "a b&c", Enabled: true}, {Key: "z", Value: "é", Enabled: true}}
	p = Resolve(in)
	if got := EncodeForm(p.BodyFields); got != "q=a+b%26c&z=%C3%A9" {
		t.Errorf("form = %q (fields keep their order)", got)
	}
	if got := headerByKey(p.Headers, "Content-Length"); got.Value != "18" {
		t.Errorf("form Content-Length = %+v", got)
	}

	in.Request.BodyType = "multipart"
	if got := headerByKey(Resolve(in).Headers, "Content-Length"); got == nil || !got.Computed || got.Value != "" {
		t.Errorf("multipart Content-Length = %+v: computed when sending", got)
	}

	in.Request.BodyType = "none"
	if headerByKey(Resolve(in).Headers, "Content-Length") != nil {
		t.Error("no body, no Content-Length")
	}
	in.Request.Headers = []Header{h("Host", "virtual.test")}
	if got := headerByKey(Resolve(in).Headers, "Host"); got.Value != "virtual.test" || got.ReadOnly {
		t.Errorf("a Host set by the user is kept: %+v", got)
	}
}

func TestWarnings(t *testing.T) {
	tests := []struct {
		name  string
		setup func(in *Input)
		want  []string
	}{
		{"none", func(in *Input) {}, nil},
		{"empty header after interpolation", func(in *Input) {
			in.Env.Vars = []Var{v("empty", "")}
			in.Request.Headers = []Header{h("X-Empty", "{{empty}}"), h("X-Blank", "")}
		}, []string{"EMPTY_HEADER:X-Empty", "EMPTY_HEADER:X-Blank"}},
		{"an ignored header is not checked", func(in *Input) {
			in.Folders[0].Headers = []Header{h("X-Empty", ""), h("X-Gone", "{{nowhere}}")}
			in.Request.Headers = []Header{ignore("X-Empty"), ignore("X-Gone")}
		}, nil},
		{"invalid URL", func(in *Input) { in.Request.URL = "api.test/users" }, []string{"INVALID_URL:api.test/users"}},
		{"not http", func(in *Input) { in.Request.URL = "ftp://files.test/a" }, []string{"INVALID_URL:ftp://files.test/a"}},
		{"empty URL", func(in *Input) { in.Request.URL = "  " }, []string{"EMPTY_URL"}},
		{"an unresolved URL is not also invalid", func(in *Input) { in.Request.URL = "{{baseUrl}}/users" },
			[]string{"UNRESOLVED_VAR:baseUrl"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := base()
			tt.setup(&in)
			if got := codes(Resolve(in)); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("warnings = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMethodAndMissingVariablesListed(t *testing.T) {
	in := base()
	in.Request.Method = " patch "
	in.Request.Body = "{{unused}}" // body type none: not sent, not missing
	in.Request.URL = "https://h/{{id}}"
	p := Resolve(in)
	if p.Method != "PATCH" {
		t.Errorf("method = %q", p.Method)
	}
	got := varByKey(p, "id")
	if got == nil || got.Source != nil || !reflect.DeepEqual(got.UsedIn, []string{"url"}) {
		t.Errorf("missing id = %+v", got)
	}
	if varByKey(p, "unused") != nil {
		t.Error("a variable in a body that is not sent is not missing")
	}
}

// --- Inherited state and the folder level --------------------------------

func TestInherited(t *testing.T) {
	in := base()
	in.Env.Vars = []Var{v("x", "env"), v("y", "env")}
	in.Folders[2].Vars = []Var{v("x", "admin")}
	in.Runtime = []Var{v("y", "rt")}
	in.Request.Vars = []Var{v("x", "req")}
	in.Folders[0].Headers = []Header{h("X-Team", "core")}
	in.Request.Headers = []Header{h("X-Team", "me"), ignore("Accept")}
	p := Resolve(in)

	if got := varByKey(Preview{Vars: p.InheritedVars}, "x"); got == nil || got.Value != "admin" {
		t.Errorf("inherited x = %+v: without the request's own row", got)
	}
	if got := varByKey(Preview{Vars: p.InheritedVars}, "y"); got == nil || got.Value != "env" {
		t.Errorf("inherited y = %+v: without runtime", got)
	}
	if got := headerByKey(p.InheritedHeaders, "X-Team"); got == nil || got.Value != "core" {
		t.Errorf("inherited X-Team = %+v", got)
	}
	if got := headerByKey(p.InheritedHeaders, "Accept"); got == nil || got.Ignored || got.Source.Kind != SourceAuto {
		t.Errorf("inherited Accept = %+v: automatic, not ignored", got)
	}
}

func TestFolderLevel(t *testing.T) {
	in := base()
	in.Request = nil
	in.Env.Vars = []Var{v("x", "env")}
	in.Folders[0].Vars = []Var{v("x", "api")}
	in.Folders[2].Vars = []Var{v("x", "admin")}
	in.Folders[0].Headers = []Header{h("X-Team", "core")}
	in.Folders[2].Headers = []Header{ignore("X-Team")}
	p := Resolve(in)

	if p.Method != "" || p.URL != "" || p.Warnings == nil || len(p.Warnings) != 0 {
		t.Errorf("a folder has no request part: %+v", p)
	}
	if got := varByKey(p, "x"); got.Value != "admin" || got.Source.Kind != SourceFolder || got.Source.Depth != 0 {
		t.Errorf("x = %+v: the edited folder is depth 0", got)
	}
	if got := varByKey(Preview{Vars: p.InheritedVars}, "x"); got.Value != "api" || got.Source.Depth != 2 {
		t.Errorf("inherited x = %+v", got)
	}
	if headerByKey(p.Headers, "User-Agent") != nil {
		t.Error("no automatic headers at the folder level")
	}
	if got := headerByKey(p.Headers, "X-Team"); !got.Ignored {
		t.Errorf("X-Team = %+v: ignored here", got)
	}
	if got := headerByKey(p.InheritedHeaders, "X-Team"); got.Ignored || got.Value != "core" {
		t.Errorf("inherited X-Team = %+v", got)
	}
}

func TestNoLayersAtAll(t *testing.T) {
	p := Resolve(Input{Request: &Request{Method: "GET", URL: "https://h", BodyType: "none"}, OS: "darwin"})
	if p.URL != "https://h" || len(p.Vars) != 0 || len(p.Warnings) != 0 {
		t.Fatalf("preview = %+v", p)
	}
	if got := headerByKey(p.Headers, "User-Agent"); !strings.Contains(got.Value, "Macintosh") {
		t.Errorf("User-Agent = %q", got.Value)
	}
}

func TestNames(t *testing.T) {
	if got := Names("{{a}} {{ b }} {{a}} {{c d}}"); !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Errorf("Names = %v", got)
	}
}

func TestListsAreNeverNil(t *testing.T) {
	in := base()
	in.Env.Vars = []Var{v("unused", "x")}
	p := Resolve(in)
	if got := varByKey(p, "unused"); got.UsedIn == nil || got.Shadowed == nil {
		t.Errorf("unused = %+v: empty lists, not nil (JSON [] for the frontend)", got)
	}
	for _, h := range p.Headers {
		if h.Shadowed == nil {
			t.Errorf("%s shadowed is nil", h.Key)
		}
	}
}
