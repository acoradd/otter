package main

import (
	"github.com/acoradd/otter/src/internal/importexport/curl"
)

// CurlDraft is a request read from a curl command, for the editor.
type CurlDraft struct {
	Draft RequestDraft `json:"draft"`
	// Ignored are the options Otter has no equivalent for, as typed.
	Ignored []string `json:"ignored"`
}

// ImportService reads requests from other tools.
type ImportService struct{}

func NewImportService() *ImportService { return &ImportService{} }

// ParseCurl reads a curl command (bash, cmd or PowerShell) into a draft. It
// writes nothing: the editor shows the draft, the user saves it.
func (s *ImportService) ParseCurl(command string) (CurlDraft, error) {
	r, err := curl.Parse(command)
	if err != nil {
		return CurlDraft{}, err
	}
	d := RequestDraft{
		Method: r.Method, URL: r.URL, BodyType: r.BodyType, Body: r.Body, BodyFile: r.BodyFile,
		EncodeURL: true, FollowRedirects: r.FollowRedirects, MaxRedirects: 10, TimeoutMS: 30000,
		Vars: []VarRow{}, Headers: make([]HeaderRow, len(r.Headers)), BodyFields: make([]BodyFieldRow, len(r.Fields)),
	}
	if r.TimeoutMS > 0 {
		d.TimeoutMS = r.TimeoutMS
	}
	for i, h := range r.Headers {
		d.Headers[i] = HeaderRow{Key: h.Key, Value: h.Value, Enabled: true}
	}
	for i, f := range r.Fields {
		d.BodyFields[i] = BodyFieldRow{Key: f.Key, Value: f.Value, File: f.File, ContentType: f.ContentType, Enabled: true}
	}
	return CurlDraft{Draft: d, Ignored: r.Ignored}, nil
}
