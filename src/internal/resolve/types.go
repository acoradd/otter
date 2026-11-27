// Package resolve turns loaded layers (runtime variables, request, folder
// chain, environment, automatic headers) into the Preview shown before a
// request is sent and used to send it: interpolation, variable and header
// resolution, warnings.
//
// It is pure: no I/O, no SQLite, no Wails, no clock. The OS is an input.
// Every rule is covered by table-driven tests (docs/specs/scope-v1.md,
// sections 4 to 6).
package resolve

// Var is a variable row of a layer.
type Var struct {
	Key     string
	Value   string
	Enabled bool
	// Secret: an environment variable whose value is masked in the UI.
	Secret bool
}

// Header is a header row of a layer. Ignore removes the inherited header of
// that name instead of setting it.
type Header struct {
	Key     string
	Value   string
	Ignore  bool
	Enabled bool
}

// Field is a field of a form URL-encoded or multipart body.
type Field struct {
	Key         string
	Value       string
	File        bool
	ContentType string
	Enabled     bool
}

// Request is the request layer: what the editor holds, saved or not.
type Request struct {
	Method   string
	URL      string
	BodyType string
	Body     string
	BodyFile string
	Fields   []Field
	Vars     []Var
	Headers  []Header
}

// Folder is a folder of the chain, with its own variables and headers.
type Folder struct {
	ID   int64
	Name string
	// Path is the names from the root down to this folder.
	Path    []string
	Vars    []Var
	Headers []Header
}

// Env is the active environment.
type Env struct {
	ID   int64
	Name string
	Vars []Var
}

// Input is every layer that applies, already loaded.
type Input struct {
	// Runtime variables of the workspace and active environment: they win
	// over everything.
	Runtime []Var
	// Folders from the root down to the request's folder (or the edited
	// folder, when Request is nil).
	Folders []Folder
	// Env is the active environment, nil when none.
	Env *Env
	// Request is the request being edited; nil to resolve at the level of
	// the last folder (the folder editor), without automatic headers.
	Request *Request
	// OS picks the User-Agent: "windows", "darwin", "linux" (runtime.GOOS).
	OS string
}

// SourceKind tells where a value comes from.
type SourceKind string

const (
	SourceRuntime SourceKind = "runtime"
	SourceRequest SourceKind = "request"
	SourceFolder  SourceKind = "folder"
	SourceEnv     SourceKind = "env"
	// SourceAuto is an automatic header (Content-Type, User-Agent…) or a
	// computed one (Host, Content-Length).
	SourceAuto SourceKind = "auto"
)

// Source is where a value comes from. Names are data: the frontend writes
// the sentence ("Inherited from users › admin").
type Source struct {
	Kind SourceKind `json:"kind"`
	ID   int64      `json:"id,omitempty"`
	// Name of the folder or environment.
	Name string `json:"name,omitempty"`
	// Path of a folder, from the root.
	Path []string `json:"path,omitempty"`
	// Depth of a folder above the current level: 0 is the request's folder.
	Depth int `json:"depth,omitempty"`
}

// Candidate is a definition hidden by a closer one.
type Candidate struct {
	Value  string `json:"value"`
	Source Source `json:"source"`
}

// ResolvedVar is a variable as it applies at the current level.
type ResolvedVar struct {
	Key string `json:"key"`
	// Value is interpolated; a missing or cyclic reference stays {{name}}.
	Value string `json:"value"`
	// RawValue is the winning definition, before interpolation.
	RawValue string `json:"rawValue"`
	// Source is nil for a variable used but defined nowhere.
	Source *Source `json:"source"`
	// Shadowed are the hidden definitions, closest first.
	Shadowed []Candidate `json:"shadowed"`
	// Secret: the value is, or holds, a secret.
	Secret bool `json:"secret"`
	// UsedIn: "url", "header:Name", "body", "field:name", "file".
	UsedIn []string `json:"usedIn"`
}

// ResolvedHeader is a header as it would be sent.
type ResolvedHeader struct {
	// Key is the interpolated name, with the casing of the winning row.
	Key      string      `json:"key"`
	RawValue string      `json:"rawValue"`
	Value    string      `json:"value"`
	Source   Source      `json:"source"`
	Shadowed []Candidate `json:"shadowed"`
	// Ignored headers are shown but not sent.
	Ignored   bool    `json:"ignored"`
	IgnoredBy *Source `json:"ignoredBy,omitempty"`
	// ReadOnly: Host and Content-Length, computed, not editable.
	ReadOnly bool `json:"readOnly"`
	// Computed: completed when sending (multipart boundary, file sizes).
	Computed bool `json:"computed"`
	Secret   bool `json:"secret"`
}

// ResolvedField is a body field with its name and value interpolated.
type ResolvedField struct {
	Key         string `json:"key"`
	Value       string `json:"value"`
	File        bool   `json:"file"`
	ContentType string `json:"contentType"`
}

// Warning is a problem the user should see; the request can still be sent.
// Codes are stable: the frontend translates them.
type Warning struct {
	Code   string            `json:"code"`
	Params map[string]string `json:"params,omitempty"`
}

// Warning codes.
const (
	WarnUnresolvedVar = "UNRESOLVED_VAR" // name
	WarnVarCycle      = "VAR_CYCLE"      // name
	WarnEmptyHeader   = "EMPTY_HEADER"   // name
	WarnInvalidURL    = "INVALID_URL"    // url
	WarnEmptyURL      = "EMPTY_URL"
)

// Preview is the request as it will be sent, with where every value comes
// from. Send builds the HTTP request from it and nothing else.
type Preview struct {
	Method     string           `json:"method"`
	URL        string           `json:"url"`
	BodyType   string           `json:"bodyType"`
	Body       string           `json:"body"`
	BodyFile   string           `json:"bodyFile"`
	BodyFields []ResolvedField  `json:"bodyFields"`
	Vars       []ResolvedVar    `json:"vars"`
	Headers    []ResolvedHeader `json:"headers"`
	// Missing are the variables used but defined nowhere, in order of use.
	Missing  []string  `json:"missing"`
	Warnings []Warning `json:"warnings"`
	// What the current level inherits, without its own rows (and without
	// runtime variables): the grey rows of the merged lists.
	InheritedVars    []ResolvedVar    `json:"inheritedVars"`
	InheritedHeaders []ResolvedHeader `json:"inheritedHeaders"`
}
