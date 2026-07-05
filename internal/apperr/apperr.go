// Package apperr defines a typed application error carrying a stable, machine-
// readable code. The code — not the human-facing message — is the API contract:
// clients branch on Code, so messages can be reworded or localized freely.
package apperr

import "errors"

type Code string

const (
	CodeUnauthorized Code = "UNAUTHORIZED"
	CodeForbidden    Code = "FORBIDDEN"
	CodeNotFound     Code = "NOT_FOUND"
	CodeInvalidInput Code = "INVALID_INPUT"
	CodeConflict     Code = "CONFLICT"
	CodeInternal     Code = "INTERNAL"
)

// Error is an application error with a stable Code and an optional wrapped cause.
type Error struct {
	Code    Code
	Message string
	Err     error
}

func (e *Error) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return string(e.Code)
}

func (e *Error) Unwrap() error { return e.Err }

func New(code Code, message string) *Error {
	return &Error{Code: code, Message: message}
}

func Wrap(code Code, message string, err error) *Error {
	return &Error{Code: code, Message: message, Err: err}
}

func Unauthorized(message string) *Error { return New(CodeUnauthorized, message) }
func Forbidden(message string) *Error    { return New(CodeForbidden, message) }
func NotFound(message string) *Error     { return New(CodeNotFound, message) }
func InvalidInput(message string) *Error { return New(CodeInvalidInput, message) }
func Conflict(message string) *Error     { return New(CodeConflict, message) }

// CodeOf returns the Code of the first *Error in err's chain, or CodeInternal
// if none is present.
func CodeOf(err error) Code {
	var appErr *Error
	if errors.As(err, &appErr) {
		return appErr.Code
	}
	return CodeInternal
}
