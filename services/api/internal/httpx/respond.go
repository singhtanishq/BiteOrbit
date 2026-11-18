// Package httpx contains the HTTP server foundation: response envelopes,
// middleware chain, session authentication and the central route registry.
package httpx

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"biteorbit/services/api/internal/apperr"
)

// Envelope is the single response shape for every endpoint:
//
//	{ "data": ..., "meta"?: ... }            on success
//	{ "error": { code, message, ... } }      on failure
type Envelope struct {
	Data      any            `json:"data,omitempty"`
	Meta      map[string]any `json:"meta,omitempty"`
	Error     *apperr.Body   `json:"error,omitempty"`
	RequestID string         `json:"request_id,omitempty"`
}

// WriteData sends a success envelope.
func WriteData(w http.ResponseWriter, r *http.Request, status int, data any) {
	writeJSON(w, r, status, Envelope{Data: data, RequestID: RequestIDFrom(r.Context())})
}

// WriteMeta sends a success envelope with pagination-style metadata.
func WriteMeta(w http.ResponseWriter, r *http.Request, data any, meta map[string]any) {
	writeJSON(w, r, http.StatusOK, Envelope{Data: data, Meta: meta, RequestID: RequestIDFrom(r.Context())})
}

// WriteError converts an error into the error envelope. Unknown errors become
// an opaque 500 without internals (no stack traces leak).
func WriteError(w http.ResponseWriter, r *http.Request, err error) {
	body, status := apperr.BodyFrom(err)
	if status >= http.StatusInternalServerError {
		slog.ErrorContext(r.Context(), "internal error",
			slog.String("request_id", RequestIDFrom(r.Context())),
			slog.String("path", r.URL.Path),
			slog.Any("err", err))
	}
	writeJSON(w, r, status, Envelope{Error: &body, RequestID: RequestIDFrom(r.Context())})
}

// Fail writes an explicit error envelope (for control-flow failures).
func Fail(w http.ResponseWriter, r *http.Request, status int, code, message string, details map[string]any) {
	writeJSON(w, r, status, Envelope{
		Error:     &apperr.Body{Code: apperr.Code(code), Message: message, Details: details},
		RequestID: RequestIDFrom(r.Context()),
	})
}

func writeJSON(w http.ResponseWriter, r *http.Request, status int, payload Envelope) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(payload)
}

// DecodeJSON strictly parses a JSON request body into dst with a size cap.
func DecodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1 MiB cap
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			return apperr.New(apperr.CodePayloadTooLarge, "Request body too large.")
		}
		return apperr.New(apperr.CodeInvalidRequest, "Request body is not valid JSON.")
	}
	return nil
}
