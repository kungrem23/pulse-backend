package health

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type pingerStub struct {
	err error
}

func (p pingerStub) Ping(context.Context) error { return p.err }

func TestReadyChecksAllDependencies(t *testing.T) {
	handler := NewHandler(pingerStub{}, pingerStub{})
	response := httptest.NewRecorder()
	handler.Ready(response, httptest.NewRequest(http.MethodGet, "/health/ready", nil))

	if response.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d", response.Code)
	}
}

func TestReadyDoesNotLeakDependencyError(t *testing.T) {
	internalError := "S3 request failed at private.internal:9000"
	handler := NewHandler(pingerStub{}, pingerStub{err: errors.New(internalError)})
	response := httptest.NewRecorder()
	handler.Ready(response, httptest.NewRequest(http.MethodGet, "/health/ready", nil))

	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("unexpected status: %d", response.Code)
	}
	if strings.Contains(response.Body.String(), internalError) || !strings.Contains(response.Body.String(), "dependency is unavailable") {
		t.Fatalf("unexpected response body: %s", response.Body.String())
	}
}

func TestReadyRejectsMissingDependency(t *testing.T) {
	handler := NewHandler(nil)
	response := httptest.NewRecorder()
	handler.Ready(response, httptest.NewRequest(http.MethodGet, "/health/ready", nil))

	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("unexpected status: %d", response.Code)
	}
}
