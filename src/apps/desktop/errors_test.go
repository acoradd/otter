package main

import (
	"errors"
	"fmt"
	"testing"

	"github.com/acoradd/otter/src/internal/apperr"
)

func TestMarshalError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{"coded", apperr.New(apperr.NotFound), `{"code":"NOT_FOUND"}`},
		{"with params", apperr.New(apperr.InvalidInput).With("field", "url"), `{"code":"INVALID_INPUT","params":{"field":"url"}}`},
		{"wrapped", fmt.Errorf("save: %w", apperr.New(apperr.NotFound)), `{"code":"NOT_FOUND"}`},
		{"plain text never leaks", errors.New("disk I/O error at /home/me"), `{"code":"INTERNAL"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := string(marshalError(tt.err)); got != tt.want {
				t.Errorf("marshalError() = %s, want %s", got, tt.want)
			}
		})
	}
}
