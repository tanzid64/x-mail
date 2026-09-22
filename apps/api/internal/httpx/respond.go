package httpx

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
)

// maxBodyBytes caps JSON request bodies. Attachment uploads use their own
// multipart path and are not affected by this limit.
const maxBodyBytes = 1 << 20 // 1 MiB

// JSON writes a JSON response. Encoding failures are logged rather than
// returned because the status line has already been committed.
func JSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if payload == nil || status == http.StatusNoContent {
		return
	}
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		slog.Error("write json response", "error", err)
	}
}

// NoContent replies 204.
func NoContent(w http.ResponseWriter) { w.WriteHeader(http.StatusNoContent) }

// Fail renders err as a JSON error response. Server-side faults are logged with
// their cause; client errors are not, so a burst of 404s cannot flood the logs.
func Fail(w http.ResponseWriter, r *http.Request, err error) {
	apiErr := AsError(err)
	if apiErr.Status >= http.StatusInternalServerError {
		slog.ErrorContext(r.Context(), "request failed",
			"error", apiErr.Error(),
			"method", r.Method,
			"path", r.URL.Path,
		)
	}
	JSON(w, apiErr.Status, map[string]any{"error": apiErr})
}

// Decode reads a JSON body into dst, translating the many ways a body can be
// malformed into one clear client-facing message.
func Decode(w http.ResponseWriter, r *http.Request, dst any) error {
	if ct := r.Header.Get("Content-Type"); ct != "" && !strings.HasPrefix(ct, "application/json") {
		return BadRequest("Content-Type must be application/json")
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()

	if err := dec.Decode(dst); err != nil {
		var syntaxErr *json.SyntaxError
		var typeErr *json.UnmarshalTypeError
		var maxErr *http.MaxBytesError

		switch {
		case errors.As(err, &syntaxErr):
			return BadRequest(fmt.Sprintf("Malformed JSON at position %d.", syntaxErr.Offset))
		case errors.As(err, &typeErr):
			return BadRequest(fmt.Sprintf("Field %q has the wrong type.", typeErr.Field))
		case errors.As(err, &maxErr):
			return BadRequest("Request body is too large.")
		case errors.Is(err, io.EOF):
			return BadRequest("Request body is empty.")
		case strings.HasPrefix(err.Error(), "json: unknown field "):
			field := strings.Trim(strings.TrimPrefix(err.Error(), "json: unknown field "), `"`)
			return BadRequest(fmt.Sprintf("Unknown field %q.", field))
		default:
			return BadRequest("Request body could not be parsed.")
		}
	}

	// A second value in the stream means the client sent more than one object.
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return BadRequest("Request body must contain a single JSON object.")
	}
	return nil
}
