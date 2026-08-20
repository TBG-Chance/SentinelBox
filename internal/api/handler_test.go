package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/TBG-Chance/SentinelBox/internal/buildinfo"
)

type healthCheckerFunc func(context.Context) (int, error)

func (f healthCheckerFunc) Check(ctx context.Context) (int, error) {
	return f(ctx)
}

func TestHealthReturnsReadyState(t *testing.T) {
	handler := NewHandler(
		testLogger(),
		healthCheckerFunc(func(context.Context) (int, error) { return 1, nil }),
		buildinfo.Info{Version: "0.1.0", Revision: "abc123", BuildTime: "2026-08-10T00:00:00Z"},
		time.Second,
	)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	request.Header.Set("X-Request-ID", "test-request-1")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	if response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("Cache-Control = %q", response.Header().Get("Cache-Control"))
	}
	if response.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("X-Content-Type-Options = %q", response.Header().Get("X-Content-Type-Options"))
	}
	if response.Header().Get("X-Request-ID") != "test-request-1" {
		t.Fatalf("X-Request-ID = %q", response.Header().Get("X-Request-ID"))
	}

	var payload healthResponse
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if payload.Status != "healthy" || payload.Database != "ready" || payload.SchemaVersion != 1 {
		t.Fatalf("unexpected health response: %#v", payload)
	}
	if payload.Version != "0.1.0" || payload.RequestID != "test-request-1" || payload.Time.IsZero() {
		t.Fatalf("missing response metadata: %#v", payload)
	}
}

func TestHealthHidesDatabaseError(t *testing.T) {
	handler := NewHandler(
		testLogger(),
		healthCheckerFunc(func(context.Context) (int, error) { return 1, errors.New("secret database path") }),
		buildinfo.Info{Version: "dev"},
		time.Second,
	)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/health", nil))

	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusServiceUnavailable)
	}
	if strings.Contains(response.Body.String(), "secret database path") {
		t.Fatal("health response exposed the internal database error")
	}
	var payload healthResponse
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Status != "degraded" || payload.Database != "unavailable" {
		t.Fatalf("unexpected degraded response: %#v", payload)
	}
}

func TestInvalidRequestIDIsReplaced(t *testing.T) {
	handler := NewHandler(testLogger(), healthCheckerFunc(func(context.Context) (int, error) { return 1, nil }), buildinfo.Info{}, time.Second)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	request.Header.Set("X-Request-ID", "invalid request id with spaces")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	id := response.Header().Get("X-Request-ID")
	if id == "" || id == request.Header.Get("X-Request-ID") || !validRequestID.MatchString(id) {
		t.Fatalf("replacement request ID = %q", id)
	}
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
