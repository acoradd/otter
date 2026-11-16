// Package apperr holds the stable error codes Go sends to the frontend.
//
// Go never returns human-readable text: a bound method fails with an *Error
// carrying a Code (and optional parameters), the desktop app marshals it as
// the call's cause, and the frontend shows the translation errors.<CODE>.
// Any other error reaches the frontend as INTERNAL and is logged.
package apperr

import "errors"

// Code is a stable, upper snake case error code, such as FOLDER_CYCLE.
// Codes are part of the contract with the frontend: never rename one.
type Code string

// Codes shared by every package. Packages declare their own codes next to
// the code that returns them.
const (
	// Internal is an unexpected failure (a bug, a broken database).
	Internal Code = "INTERNAL"
	// NotFound is a workspace, folder, request or environment that does not exist.
	NotFound Code = "NOT_FOUND"
	// InvalidInput is an argument the frontend should never have sent.
	InvalidInput Code = "INVALID_INPUT"
)

// Error is an error with a stable code, and parameters for its message
// (the translation may use them: {name}, {count}…).
type Error struct {
	Code   Code
	Params map[string]any
	// Err is the underlying cause, for logs only: it never reaches the frontend.
	Err error
}

// New returns an error with a code and no parameters.
func New(code Code) *Error {
	return &Error{Code: code}
}

// Wrap returns an error with a code, keeping err as its cause for logs.
func Wrap(code Code, err error) *Error {
	return &Error{Code: code, Err: err}
}

// With returns a copy of e with a parameter added.
func (e *Error) With(key string, value any) *Error {
	params := make(map[string]any, len(e.Params)+1)
	for k, v := range e.Params {
		params[k] = v
	}
	params[key] = value
	return &Error{Code: e.Code, Params: params, Err: e.Err}
}

func (e *Error) Error() string {
	if e.Err != nil {
		return string(e.Code) + ": " + e.Err.Error()
	}
	return string(e.Code)
}

func (e *Error) Unwrap() error {
	return e.Err
}

// Is matches errors with the same code, so errors.Is(err, apperr.New(c)) works.
func (e *Error) Is(target error) bool {
	var t *Error
	return errors.As(target, &t) && t.Code == e.Code
}

// CodeOf returns the code of err, Internal when it carries none.
func CodeOf(err error) Code {
	var e *Error
	if errors.As(err, &e) {
		return e.Code
	}
	return Internal
}
