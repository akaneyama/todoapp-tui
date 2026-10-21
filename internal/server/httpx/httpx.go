// Package httpx holds small JSON request/response helpers shared by
// handlers and middleware.
package httpx

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"todoapp/internal/models"
)

// WriteJSON writes v as JSON with the given status code.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if v == nil {
		return
	}
	if err := json.NewEncoder(w).Encode(v); err != nil {
		// Headers are already sent; nothing useful to do but log.
		slog.Default().Warn("write json response", "err", err)
	}
}

// WriteError writes the standard error envelope.
func WriteError(w http.ResponseWriter, status int, code, message string) {
	WriteJSON(w, status, models.ErrorResponse{Error: models.APIError{Code: code, Message: message}})
}

// WriteFieldError writes a validation error pointing at a field.
func WriteFieldError(w http.ResponseWriter, field, message string) {
	WriteJSON(w, http.StatusBadRequest, models.ErrorResponse{Error: models.APIError{
		Code: models.CodeValidation, Message: message, Field: field,
	}})
}

// DecodeError is returned by DecodeJSON and carries the HTTP status to use.
type DecodeError struct {
	Status  int
	Code    string
	Message string
}

func (e *DecodeError) Error() string { return e.Message }

// Write sends the decode error to the client.
func (e *DecodeError) Write(w http.ResponseWriter) { WriteError(w, e.Status, e.Code, e.Message) }

// DecodeJSON decodes exactly one JSON object from the request body into dst.
// Unknown fields are rejected. The body size is expected to be capped by
// the MaxBody middleware; exceeding it yields 413.
func DecodeJSON(r *http.Request, dst any) *DecodeError {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		var maxErr *http.MaxBytesError
		var syntaxErr *json.SyntaxError
		var typeErr *json.UnmarshalTypeError
		switch {
		case errors.As(err, &maxErr):
			return &DecodeError{http.StatusRequestEntityTooLarge, models.CodeTooLarge,
				fmt.Sprintf("request body must not exceed %d bytes", maxErr.Limit)}
		case errors.Is(err, io.EOF):
			return &DecodeError{http.StatusBadRequest, models.CodeBadRequest, "request body must not be empty"}
		case errors.As(err, &syntaxErr), errors.Is(err, io.ErrUnexpectedEOF):
			return &DecodeError{http.StatusBadRequest, models.CodeBadRequest, "request body is not valid JSON"}
		case errors.As(err, &typeErr):
			return &DecodeError{http.StatusBadRequest, models.CodeBadRequest,
				fmt.Sprintf("field %q has the wrong type", typeErr.Field)}
		default:
			// Includes "json: unknown field".
			return &DecodeError{http.StatusBadRequest, models.CodeBadRequest, err.Error()}
		}
	}
	if dec.More() {
		return &DecodeError{http.StatusBadRequest, models.CodeBadRequest, "request body must contain a single JSON object"}
	}
	return nil
}
