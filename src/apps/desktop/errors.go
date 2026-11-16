package main

import (
	"encoding/json"
	"errors"
	"log/slog"

	"github.com/acoradd/otter/src/internal/apperr"
)

// errorCause is what the frontend receives as the "cause" of a rejected call.
type errorCause struct {
	Code   apperr.Code    `json:"code"`
	Params map[string]any `json:"params,omitempty"`
}

// marshalError turns any error returned by a bound method into a stable code.
// Unexpected errors are logged and reported as INTERNAL: Go never sends
// human-readable text to the frontend.
func marshalError(err error) []byte {
	cause := errorCause{Code: apperr.Internal}
	var e *apperr.Error
	if errors.As(err, &e) {
		cause = errorCause{Code: e.Code, Params: e.Params}
		if e.Code == apperr.Internal {
			slog.Error("internal error", "err", err)
		}
	} else {
		slog.Error("unexpected error", "err", err)
	}
	b, _ := json.Marshal(cause)
	return b
}
