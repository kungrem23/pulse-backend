package httpx

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
)

// MaxJSONBodyBytes bounds JSON request bodies; larger bodies fail to decode.
const MaxJSONBodyBytes = 16 << 10

// DecodeJSON reads exactly one JSON value and rejects unknown fields.
func DecodeJSON(r *http.Request, destination any) error {
	decoder := json.NewDecoder(io.LimitReader(r.Body, MaxJSONBodyBytes))
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(destination); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("request body must contain a single JSON value")
	}
	return nil
}

type APIError struct {
	Code    string            `json:"code"`
	Message string            `json:"message"`
	Details map[string]string `json:"details,omitempty"`
}

type ErrorResponse struct {
	Error APIError `json:"error"`
}

func WriteJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func Error(w http.ResponseWriter, status int, message string) {
	codes := map[int]string{
		http.StatusBadRequest:            "bad_request",
		http.StatusUnauthorized:          "unauthorized",
		http.StatusForbidden:             "forbidden",
		http.StatusNotFound:              "not_found",
		http.StatusConflict:              "conflict",
		http.StatusRequestEntityTooLarge: "payload_too_large",
		http.StatusUnsupportedMediaType:  "unsupported_media_type",
		http.StatusUnprocessableEntity:   "validation_failed",
		http.StatusTooManyRequests:       "too_many_requests",
		http.StatusInternalServerError:   "internal_error",
		http.StatusNotImplemented:        "not_implemented",
		http.StatusServiceUnavailable:    "service_unavailable",
	}
	code, ok := codes[status]
	if !ok {
		code = "request_failed"
	}
	ErrorCode(w, status, code, message)
}

func ErrorCode(w http.ResponseWriter, status int, code, message string) {
	WriteJSON(w, status, ErrorResponse{Error: APIError{Code: code, Message: message}})
}

func ValidationFailed(w http.ResponseWriter, field, message string) {
	WriteJSON(w, http.StatusUnprocessableEntity, ErrorResponse{Error: APIError{
		Code:    "validation_failed",
		Message: field + " " + message,
		Details: map[string]string{field: message},
	}})
}

func NotImplemented(operation string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		WriteJSON(w, http.StatusNotImplemented, ErrorResponse{Error: APIError{
			Code:    "not_implemented",
			Message: "not implemented",
			Details: map[string]string{"operation": operation},
		}})
	}
}
