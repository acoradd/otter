package resolve

import (
	"regexp"
	"strings"
)

// placeholder is {{name}}, spaces tolerated, name in [\w.-]+.
var placeholder = regexp.MustCompile(`\{\{\s*([\w.\-]+)\s*\}\}`)

// MaxDepth is how deep a value may reference other values.
const MaxDepth = 10

// Names returns the variables a text references, in order, without repeats.
func Names(text string) []string {
	var names []string
	seen := map[string]bool{}
	for _, m := range placeholder.FindAllStringSubmatch(text, -1) {
		if !seen[m[1]] {
			seen[m[1]] = true
			names = append(names, m[1])
		}
	}
	return names
}

// definition is the winning definition of a variable, and the hidden ones.
type definition struct {
	raw      string
	secret   bool
	source   Source
	shadowed []Candidate
}

// value is a variable once interpolated.
type value struct {
	text    string
	secret  bool
	missing []string // used but undefined, in order
	cyclic  []string // variables on a cycle (or too deep)
}

// interpolator expands {{name}} with a set of definitions, memoising values.
type interpolator struct {
	defs  map[string]*definition
	memo  map[string]*value
	state map[string]int // 1: being expanded, 2: done
}

func newInterpolator(defs map[string]*definition) *interpolator {
	return &interpolator{defs: defs, memo: map[string]*value{}, state: map[string]int{}}
}

// expand interpolates text. Missing and cyclic references stay as written.
func (in *interpolator) expand(text string) value {
	return in.expandAt(text, 0)
}

func (in *interpolator) expandAt(text string, depth int) value {
	var out value
	out.text = placeholder.ReplaceAllStringFunc(text, func(m string) string {
		name := placeholder.FindStringSubmatch(m)[1]
		if _, ok := in.defs[name]; !ok {
			out.missing = appendOnce(out.missing, name)
			return m
		}
		v := in.valueAt(name, depth+1)
		if v == nil { // a cycle, or too deep
			out.cyclic = appendOnce(out.cyclic, name)
			return m
		}
		out.secret = out.secret || v.secret
		out.missing = appendAll(out.missing, v.missing)
		out.cyclic = appendAll(out.cyclic, v.cyclic)
		return v.text
	})
	return out
}

// valueAt is the interpolated value of a defined variable, nil when it sits
// on a cycle or past MaxDepth.
func (in *interpolator) valueAt(name string, depth int) *value {
	if v, ok := in.memo[name]; ok {
		return v
	}
	if in.state[name] == 1 || depth > MaxDepth {
		return nil
	}
	in.state[name] = 1
	def := in.defs[name]
	v := in.expandAt(def.raw, depth)
	v.secret = v.secret || def.secret
	in.state[name] = 2
	in.memo[name] = &v
	return &v
}

// valueOf is the interpolated value of a defined variable at the top level;
// a variable on a cycle keeps its raw value and is reported.
func (in *interpolator) valueOf(name string) value {
	if v := in.valueAt(name, 0); v != nil {
		return *v
	}
	return value{text: in.defs[name].raw, secret: in.defs[name].secret, cyclic: []string{name}}
}

func appendOnce(list []string, s string) []string {
	for _, x := range list {
		if x == s {
			return list
		}
	}
	return append(list, s)
}

func appendAll(list, more []string) []string {
	for _, s := range more {
		list = appendOnce(list, s)
	}
	return list
}

// EncodeForm encodes form fields as application/x-www-form-urlencoded, in
// their order (url.Values would sort them). Send uses it too.
func EncodeForm(fields []ResolvedField) string {
	var b strings.Builder
	for i, f := range fields {
		if i > 0 {
			b.WriteByte('&')
		}
		b.WriteString(formEscape(f.Key))
		b.WriteByte('=')
		b.WriteString(formEscape(f.Value))
	}
	return b.String()
}

func formEscape(s string) string {
	const hex = "0123456789ABCDEF"
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case 'a' <= c && c <= 'z', 'A' <= c && c <= 'Z', '0' <= c && c <= '9', c == '-', c == '_', c == '.', c == '~':
			b.WriteByte(c)
		case c == ' ':
			b.WriteByte('+')
		default:
			b.WriteByte('%')
			b.WriteByte(hex[c>>4])
			b.WriteByte(hex[c&15])
		}
	}
	return b.String()
}
