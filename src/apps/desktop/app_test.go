package main

import (
	"testing"

	"github.com/acoradd/otter/src/internal/version"
)

func TestGetAppInfo(t *testing.T) {
	got := NewAppService().GetAppInfo().Version
	if got == version.Dev {
		return
	}
	if _, err := version.Parse(got); err != nil {
		t.Errorf("version.txt holds %q: want dev or MAJOR.MINOR.PATCH", got)
	}
}
