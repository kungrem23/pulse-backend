package httpx

import (
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strings"
)

// MaxJSONBodyBytes bounds JSON request bodies; larger bodies fail to decode.
const MaxJSONBodyBytes = 16 << 10

// RequestIDHeader carries the request ID set by middleware.RequestID. Error
// responses copy it into request_id.
const RequestIDHeader = "X-Request-ID"

var (
	ErrUnsupportedContentType = errors.New("request body must be application/json")
	ErrBodyTooLarge           = errors.New("request body is too large")
)

// DecodeJSON reads exactly one JSON value and rejects unknown fields.
// An explicit Content-Type other than application/json (charset utf-8 only)
// is rejected; a missing one is accepted. Browsers send text/plain for a
// plain string body, so that case is rejected.
func DecodeJSON(r *http.Request, destination any) error {
	if err := checkJSONContentType(r.Header.Get("Content-Type")); err != nil {
		return err
	}
	body := &limitedReader{reader: r.Body, remaining: MaxJSONBodyBytes}
	decoder := json.NewDecoder(body)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(destination); err != nil {
		if body.exceeded {
			return ErrBodyTooLarge
		}
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if body.exceeded {
			return ErrBodyTooLarge
		}
		return errors.New("request body must contain a single JSON value")
	}
	return nil
}

func checkJSONContentType(value string) error {
	if value == "" {
		return nil
	}
	mediaType, params, err := mime.ParseMediaType(value)
	if err != nil || mediaType != "application/json" {
		return ErrUnsupportedContentType
	}
	if charset, ok := params["charset"]; ok && !strings.EqualFold(charset, "utf-8") {
		return ErrUnsupportedContentType
	}
	return nil
}

// limitedReader fails instead of silently truncating, so an oversized body is
// not decoded as a shorter valid document.
type limitedReader struct {
	reader    io.Reader
	remaining int64
	exceeded  bool
}

func (l *limitedReader) Read(p []byte) (int, error) {
	if l.remaining <= 0 {
		var probe [1]byte
		if n, _ := l.reader.Read(probe[:]); n > 0 {
			l.exceeded = true
			return 0, ErrBodyTooLarge
		}
		return 0, io.EOF
	}
	if int64(len(p)) > l.remaining {
		p = p[:l.remaining]
	}
	n, err := l.reader.Read(p)
	l.remaining -= int64(n)
	return n, err
}

type APIError struct {
	Code    string            `json:"code"`
	Message string            `json:"message"`
	Details map[string]string `json:"details,omitempty"`
}

type ErrorResponse struct {
	Error     APIError `json:"error"`
	RequestID string   `json:"request_id,omitempty"`
}

func WriteJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

// statusCodes are the default error codes; ErrorCode sets a specific one.
var statusCodes = map[int]string{
	http.StatusBadRequest:            "bad_request",
	http.StatusUnauthorized:          "unauthorized",
	http.StatusForbidden:             "forbidden",
	http.StatusNotFound:              "not_found",
	http.StatusMethodNotAllowed:      "method_not_allowed",
	http.StatusConflict:              "conflict",
	http.StatusRequestEntityTooLarge: "payload_too_large",
	http.StatusUnsupportedMediaType:  "unsupported_media_type",
	http.StatusUnprocessableEntity:   "validation_failed",
	http.StatusTooManyRequests:       "too_many_requests",
	http.StatusInternalServerError:   "internal_error",
	http.StatusNotImplemented:        "not_implemented",
	http.StatusServiceUnavailable:    "service_unavailable",
}

func Error(w http.ResponseWriter, status int, message string) {
	code, ok := statusCodes[status]
	if !ok {
		code = "request_failed"
	}
	ErrorCode(w, status, code, message)
}

func ErrorCode(w http.ResponseWriter, status int, code, message string) {
	writeError(w, status, APIError{Code: code, Message: message})
}

func ValidationFailed(w http.ResponseWriter, field, message string) {
	writeError(w, http.StatusUnprocessableEntity, APIError{
		Code:    "validation_failed",
		Message: field + " " + message,
		Details: map[string]string{field: message},
	})
}

func NotImplemented(operation string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		writeError(w, http.StatusNotImplemented, APIError{
			Code:    "not_implemented",
			Message: "not implemented",
			Details: map[string]string{"operation": operation},
		})
	}
}

// writeError is the single place that builds the error envelope.
func writeError(w http.ResponseWriter, status int, apiError APIError) {
	WriteJSON(w, status, ErrorResponse{Error: apiError, RequestID: w.Header().Get(RequestIDHeader)})
}
