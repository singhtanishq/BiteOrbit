// Package apperr defines the application error model: stable machine codes,
// human messages and HTTP status mapping.
package apperr

import (
	"errors"
	"net/http"
)

type Code string

const (
	CodeInvalidRequest     Code = "invalid_request"
	CodeValidation         Code = "validation_failed"
	CodeUnauthorized       Code = "unauthorized"
	CodeForbidden          Code = "forbidden"
	CodeNotFound           Code = "not_found"
	CodeConflict           Code = "conflict"
	CodeInvalidState       Code = "invalid_state_transition"
	CodeIdempotencyReplay  Code = "idempotency_replayed"
	CodeRateLimited        Code = "rate_limited"
	CodePayloadTooLarge    Code = "payload_too_large"
	CodeUnprocessable      Code = "unprocessable"
	CodeServiceUnavailable Code = "service_unavailable"
	CodeInternal           Code = "internal_error"
)

// Body is the JSON error shape exposed to clients.
type Body struct {
	Code    Code           `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
}

type AppError struct {
	Status  int
	Code    Code
	Message string
	Details map[string]any
}

func (e *AppError) Error() string { return string(e.Code) + ": " + e.Message }

// New creates an AppError with the conventional status for the code.
func New(code Code, message string) *AppError {
	return &AppError{Status: statusFor(code), Code: code, Message: message}
}

// WithDetails attaches structured, non-sensitive details.
func (e *AppError) WithDetails(details map[string]any) *AppError {
	e.Details = details
	return e
}

func statusFor(code Code) int {
	switch code {
	case CodeInvalidRequest, CodeValidation:
		return http.StatusBadRequest
	case CodeUnauthorized:
		return http.StatusUnauthorized
	case CodeForbidden:
		return http.StatusForbidden
	case CodeNotFound:
		return http.StatusNotFound
	case CodeConflict, CodeInvalidState, CodeIdempotencyReplay:
		return http.StatusConflict
	case CodeRateLimited:
		return http.StatusTooManyRequests
	case CodePayloadTooLarge:
		return http.StatusRequestEntityTooLarge
	case CodeUnprocessable:
		return http.StatusUnprocessableEntity
	case CodeServiceUnavailable:
		return http.StatusServiceUnavailable
	default:
		return http.StatusInternalServerError
	}
}

// BodyFrom maps any error to (error body, http status). Unknown errors are
// sanitised to an opaque internal error so internals never reach clients.
func BodyFrom(err error) (Body, int) {
	var ae *AppError
	if errors.As(err, &ae) {
		return Body{Code: ae.Code, Message: ae.Message, Details: ae.Details}, ae.Status
	}
	return Body{Code: CodeInternal, Message: "Something went wrong. Please try again."}, http.StatusInternalServerError
}
