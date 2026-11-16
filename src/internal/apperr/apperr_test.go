package apperr

import (
	"errors"
	"fmt"
	"io"
	"testing"
)

func TestCodeOf(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want Code
	}{
		{"coded", New(NotFound), NotFound},
		{"wrapped by fmt", fmt.Errorf("loading: %w", New(InvalidInput)), InvalidInput},
		{"with a cause", Wrap(NotFound, io.EOF), NotFound},
		{"plain", io.EOF, Internal},
		{"nil", nil, Internal},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CodeOf(tt.err); got != tt.want {
				t.Errorf("CodeOf() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestErrorMessageAndUnwrap(t *testing.T) {
	err := Wrap(Internal, io.EOF)
	if err.Error() != "INTERNAL: EOF" {
		t.Errorf("Error() = %q", err.Error())
	}
	if !errors.Is(err, io.EOF) {
		t.Error("the cause should be reachable with errors.Is")
	}
	if New(NotFound).Error() != "NOT_FOUND" {
		t.Errorf("Error() = %q", New(NotFound).Error())
	}
}

func TestIsMatchesByCode(t *testing.T) {
	err := fmt.Errorf("move: %w", New(NotFound).With("id", 3))
	if !errors.Is(err, New(NotFound)) {
		t.Error("same code should match")
	}
	if errors.Is(err, New(InvalidInput)) {
		t.Error("another code should not match")
	}
}

func TestWithCopies(t *testing.T) {
	base := New(InvalidInput).With("field", "url")
	more := base.With("max", 10)
	if len(base.Params) != 1 || len(more.Params) != 2 {
		t.Errorf("With should not change the original: %v, %v", base.Params, more.Params)
	}
	if more.Params["field"] != "url" || more.Params["max"] != 10 {
		t.Errorf("params = %v", more.Params)
	}
}
