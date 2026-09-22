// Package httpx holds the single request/response conventions shared by every
// HTTP handler: JSON encoding, body decoding, and the one place where an error
// becomes a status code.
package httpx

import (
	"errors"
	"fmt"
	"net/http"
)

// Error is an application error carrying everything needed to render an API
// response. Handlers return these instead of writing status codes themselves.
type Error struct {
	Status  int               `json:"-"`
	Code    string            `json:"code"`
	Message string            `json:"message"`
	Fields  map[string]string `json:"fields,omitempty"`
	err     error
}

func (e *Error) Error() string {
	if e.err != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.err)
	}
	return e.Message
}

func (e *Error) Unwrap() error { return e.err }

// Wrap attaches an underlying cause. The cause is logged but never sent to the
// client, so internal details cannot leak through an API response.
func (e *Error) Wrap(err error) *Error {
	clone := *e
	clone.err = err
	return &clone
}

// WithFields attaches per-field validation messages.
func (e *Error) WithFields(fields map[string]string) *Error {
	clone := *e
	clone.Fields = fields
	return &clone
}

func newError(status int, code, message string) *Error {
	return &Error{Status: status, Code: code, Message: message}
}

func BadRequest(message string) *Error {
	return newError(http.StatusBadRequest, "bad_request", message)
}
func Unauthorized(message string) *Error {
	return newError(http.StatusUnauthorized, "unauthorized", message)
}
func Forbidden(message string) *Error { return newError(http.StatusForbidden, "forbidden", message) }
func NotFound(message string) *Error  { return newError(http.StatusNotFound, "not_found", message) }
func Conflict(message string) *Error  { return newError(http.StatusConflict, "conflict", message) }
func TooManyRequests(message string) *Error {
	return newError(http.StatusTooManyRequests, "rate_limited", message)
}
func UnprocessableEntity(message string) *Error {
	return newError(http.StatusUnprocessableEntity, "validation_failed", message)
}
func Internal(err error) *Error {
	return (&Error{
		Status:  http.StatusInternalServerError,
		Code:    "internal_error",
		Message: "Something went wrong. Please try again.",
	}).Wrap(err)
}

// AsError converts any error into an *Error, treating unrecognised errors as
// internal failures so that no handler can accidentally expose a raw message.
func AsError(err error) *Error {
	var apiErr *Error
	if errors.As(err, &apiErr) {
		return apiErr
	}
	return Internal(err)
}
