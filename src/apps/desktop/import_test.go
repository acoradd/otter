package main

import (
	"reflect"
	"testing"

	"github.com/acoradd/otter/src/internal/importexport/curl"
)

func TestParseCurl(t *testing.T) {
	s := NewImportService()
	got := must(s.ParseCurl(`curl -k -L --max-time 3 -H 'X-A: 1' -F 'f=@/tmp/a.png' https://api.test/up`))
	want := CurlDraft{
		Draft: RequestDraft{
			Method: "POST", URL: "https://api.test/up", BodyType: "multipart", EncodeURL: true, FollowRedirects: true,
			MaxRedirects: 10, TimeoutMS: 3000, Vars: []VarRow{},
			Headers:    []HeaderRow{{Key: "X-A", Value: "1", Enabled: true}},
			BodyFields: []BodyFieldRow{{Key: "f", Value: "/tmp/a.png", File: true, Enabled: true}},
		},
		Ignored: []string{"-k"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got  %+v\nwant %+v", got, want)
	}
	if d := must(s.ParseCurl("curl https://h")).Draft; d.TimeoutMS != 30000 || d.FollowRedirects {
		t.Errorf("defaults: %+v", d)
	}
	_, err := s.ParseCurl("not curl")
	wantCode(t, err, curl.CodeInvalid)
}
