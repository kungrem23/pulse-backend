package httpx

import (
	"encoding/json"
	"net/http"
)

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

func NotImplemented(operation string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		WriteJSON(w, http.StatusNotImplemented, ErrorResponse{Error: APIError{
			Code:    "not_implemented",
			Message: "not implemented",
			Details: map[string]string{"operation": operation},
		}})
	}
}
