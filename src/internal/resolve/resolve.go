package resolve

import (
	"net/url"
	"strings"
)

// Resolve applies every layer and returns the preview of the current level:
// the request when Input.Request is set, else the last folder.
func Resolve(in Input) Preview {
	p := Preview{
		BodyFields: []ResolvedField{}, Missing: []string{}, Warnings: []Warning{},
	}

	// Variables: what is inherited (no runtime, no current level), then all.
	p.InheritedVars = resolveVars(varLayers(in, false, false)).list
	all := resolveVars(varLayers(in, true, true))
	in2 := newInterpolator(all.defs)

	used := map[string][]string{} // variable → where the request uses it
	use := func(where, text string) value {
		for _, n := range Names(text) {
			used[n] = append(used[n], where)
		}
		v := in2.expand(text)
		p.Missing = appendAll(p.Missing, v.missing)
		for _, c := range v.cyclic {
			addWarning(&p, WarnVarCycle, "name", c)
		}
		return v
	}

	// Headers: folders root first, then automatic ones, then the request.
	chain := folderHeaderLayers(in)
	current := len(chain)
	if in.Request == nil && current > 0 {
		current-- // the edited folder is the current level
	}
	inherited := newHeaderState()
	for _, l := range chain[:current] {
		inherited.apply(l.rows, l.source, in2)
	}
	state := inherited.clone()
	for _, l := range chain[current:] {
		state.apply(l.rows, l.source, in2)
	}

	if r := in.Request; r != nil {
		p.Method = strings.ToUpper(strings.TrimSpace(r.Method))
		p.BodyType = r.BodyType
		u := use("url", r.URL)
		p.URL = u.text
		switch r.BodyType {
		case "json", "xml", "text":
			p.Body = use("body", r.Body).text
		case "binary":
			p.BodyFile = use("file", r.BodyFile).text
		case "form-urlencoded", "multipart":
			for _, f := range r.Fields {
				if !f.Enabled || (f.File && r.BodyType != "multipart") {
					continue
				}
				key := use("field:"+f.Key, f.Key).text
				val := use("field:"+f.Key, f.Value).text
				p.BodyFields = append(p.BodyFields, ResolvedField{Key: key, Value: val, File: f.File, ContentType: strings.TrimSpace(f.ContentType)})
			}
		}

		// Automatic headers apply unless a folder already defined them.
		auto := Source{Kind: SourceAuto}
		for _, h := range autoHeaders(r, in.OS, p.BodyFile) {
			if !state.has(h[0]) {
				state.add(h[0], h[1], auto)
				inherited.add(h[0], h[1], auto)
			}
		}
		if ct := state.get("Content-Type"); ct != nil && ct.Source.Kind == SourceAuto && r.BodyType == "multipart" {
			ct.Computed = true
			if ih := inherited.get("Content-Type"); ih != nil {
				ih.Computed = true
			}
		}
		state.apply(r.Headers, Source{Kind: SourceRequest}, in2)

		switch {
		case strings.TrimSpace(r.URL) == "":
			addWarning(&p, WarnEmptyURL, "", "")
		case len(u.missing) == 0 && len(u.cyclic) == 0 && !validURL(p.URL):
			addWarning(&p, WarnInvalidURL, "url", p.URL)
		}
		addComputedHeaders(&p, state)
	}

	// Values of the headers, with their own variables.
	quiet := func(_ string, text string) value { return in2.expand(text) }
	p.Headers = state.resolve(&p, use, quiet, true)
	p.InheritedHeaders = inherited.resolve(&p, quiet, quiet, false)

	// Variables as they apply here, with where the request uses them.
	usedIn := func(name string) []string {
		if list := used[name]; list != nil {
			return list
		}
		return []string{}
	}
	for _, v := range all.list {
		v.UsedIn = usedIn(v.Key)
		p.Vars = append(p.Vars, v)
	}
	for _, name := range p.Missing {
		p.Vars = append(p.Vars, ResolvedVar{Key: name, Value: "{{" + name + "}}", Shadowed: []Candidate{}, UsedIn: usedIn(name)})
		addWarning(&p, WarnUnresolvedVar, "name", name)
	}
	if p.Vars == nil {
		p.Vars = []ResolvedVar{}
	}
	return p
}

func addWarning(p *Preview, code, key, value string) {
	w := Warning{Code: code}
	if key != "" {
		w.Params = map[string]string{key: value}
	}
	for _, x := range p.Warnings {
		if x.Code == w.Code && x.Params[key] == value {
			return
		}
	}
	p.Warnings = append(p.Warnings, w)
}

func validURL(s string) bool {
	u, err := url.Parse(s)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}

// --- Variables -----------------------------------------------------------

type varLayer struct {
	rows   []Var
	source Source
}

// varLayers lists the variable layers, highest priority first: runtime,
// request (or the edited folder), folders closest first, environment.
func varLayers(in Input, runtime, current bool) []varLayer {
	var layers []varLayer
	if runtime && len(in.Runtime) > 0 {
		layers = append(layers, varLayer{in.Runtime, Source{Kind: SourceRuntime}})
	}
	folders := in.Folders
	if in.Request != nil {
		if current {
			layers = append(layers, varLayer{in.Request.Vars, Source{Kind: SourceRequest}})
		}
	} else if len(folders) > 0 && !current {
		folders = folders[:len(folders)-1]
	}
	for i := len(folders) - 1; i >= 0; i-- {
		f := folders[i]
		depth := len(in.Folders) - 1 - i
		layers = append(layers, varLayer{f.Vars, Source{Kind: SourceFolder, ID: f.ID, Name: f.Name, Path: f.Path, Depth: depth}})
	}
	if in.Env != nil {
		layers = append(layers, varLayer{in.Env.Vars, Source{Kind: SourceEnv, ID: in.Env.ID, Name: in.Env.Name}})
	}
	return layers
}

type varSet struct {
	defs map[string]*definition
	list []ResolvedVar
}

// resolveVars picks, for each name, the highest layer defining it; within a
// layer the last enabled row wins. Values are interpolated with the set.
func resolveVars(layers []varLayer) varSet {
	set := varSet{defs: map[string]*definition{}}
	var order []string
	for _, l := range layers {
		// The last enabled row of a layer wins over the others of that layer.
		rows := map[string]Var{}
		var names []string
		for _, v := range l.rows {
			k := strings.TrimSpace(v.Key)
			if !v.Enabled || k == "" {
				continue
			}
			if _, ok := rows[k]; !ok {
				names = append(names, k)
			}
			rows[k] = v
		}
		for _, k := range names {
			v := rows[k]
			if d, ok := set.defs[k]; ok {
				d.shadowed = append(d.shadowed, Candidate{Value: v.Value, Source: l.source})
				continue
			}
			set.defs[k] = &definition{raw: v.Value, secret: v.Secret, source: l.source, shadowed: []Candidate{}}
			order = append(order, k)
		}
	}
	in := newInterpolator(set.defs)
	for _, k := range order {
		d := set.defs[k]
		v := in.valueOf(k)
		src := d.source
		set.list = append(set.list, ResolvedVar{
			Key: k, Value: v.text, RawValue: d.raw, Source: &src, Shadowed: d.shadowed, Secret: v.secret, UsedIn: []string{},
		})
	}
	if set.list == nil {
		set.list = []ResolvedVar{}
	}
	return set
}

// --- Headers -------------------------------------------------------------

type headerLayer struct {
	rows   []Header
	source Source
}

// folderHeaderLayers lists the folder header layers, root first.
func folderHeaderLayers(in Input) []headerLayer {
	layers := make([]headerLayer, len(in.Folders))
	for i, f := range in.Folders {
		layers[i] = headerLayer{f.Headers, Source{Kind: SourceFolder, ID: f.ID, Name: f.Name, Path: f.Path, Depth: len(in.Folders) - 1 - i}}
	}
	return layers
}

// headerState is the headers so far, in order of first appearance, matched
// by name without case.
type headerState struct {
	order []string
	byKey map[string]*ResolvedHeader
}

func newHeaderState() *headerState {
	return &headerState{byKey: map[string]*ResolvedHeader{}}
}

func (s *headerState) clone() *headerState {
	c := newHeaderState()
	for _, k := range s.order {
		h := *s.byKey[k]
		h.Shadowed = append([]Candidate{}, h.Shadowed...)
		c.order = append(c.order, k)
		c.byKey[k] = &h
	}
	return c
}

func (s *headerState) has(name string) bool { return s.byKey[strings.ToLower(name)] != nil }

func (s *headerState) get(name string) *ResolvedHeader { return s.byKey[strings.ToLower(name)] }

func (s *headerState) add(name, value string, src Source) {
	k := strings.ToLower(name)
	s.order = append(s.order, k)
	s.byKey[k] = &ResolvedHeader{Key: name, RawValue: value, Source: src, Shadowed: []Candidate{}}
}

// apply layers rows on the state: set adds or replaces (the old value is
// shadowed, an earlier ignore is undone), ignore removes what is inherited.
func (s *headerState) apply(rows []Header, src Source, in *interpolator) {
	for _, r := range rows {
		if !r.Enabled || strings.TrimSpace(r.Key) == "" {
			continue
		}
		name := in.expand(strings.TrimSpace(r.Key)).text
		k := strings.ToLower(name)
		h := s.byKey[k]
		if r.Ignore {
			if h != nil && !h.Ignored {
				by := src
				h.Ignored, h.IgnoredBy = true, &by
			}
			continue
		}
		if h == nil {
			s.add(name, r.Value, src)
			continue
		}
		h.Shadowed = append([]Candidate{{Value: h.RawValue, Source: h.Source}}, h.Shadowed...)
		h.Key, h.RawValue, h.Source = name, r.Value, src
		h.Ignored, h.IgnoredBy = false, nil
	}
}

// resolve interpolates the values: with use for headers that are sent (their
// variables count as used), quiet for ignored ones. When warn, an empty value
// of a header that is sent is reported.
func (s *headerState) resolve(p *Preview, use, quiet func(where, text string) value, warn bool) []ResolvedHeader {
	out := make([]ResolvedHeader, 0, len(s.order))
	for _, k := range s.order {
		h := *s.byKey[k]
		if !h.ReadOnly {
			expand := use
			if h.Ignored {
				expand = quiet
			}
			v := expand("header:"+h.Key, h.RawValue)
			h.Value, h.Secret = v.text, v.secret
		}
		if warn && !h.Ignored && !h.Computed && strings.TrimSpace(h.Value) == "" {
			addWarning(p, WarnEmptyHeader, "name", h.Key)
		}
		out = append(out, h)
	}
	return out
}

// addComputedHeaders adds Host and Content-Length, read-only, unless set.
func addComputedHeaders(p *Preview, s *headerState) {
	auto := Source{Kind: SourceAuto}
	if u, err := url.Parse(p.URL); err == nil && u.Host != "" && !s.has("Host") {
		s.add("Host", u.Host, auto)
		h := s.get("Host")
		h.ReadOnly, h.Value = true, u.Host
	}
	if s.has("Content-Length") {
		return
	}
	length, computed := "", false
	switch p.BodyType {
	case "json", "xml", "text":
		length = itoa(len(p.Body))
	case "form-urlencoded":
		length = itoa(len(EncodeForm(p.BodyFields)))
	case "multipart", "binary":
		computed = true
	default:
		return
	}
	s.add("Content-Length", length, auto)
	h := s.get("Content-Length")
	h.ReadOnly, h.Computed, h.Value = true, computed, length
}
