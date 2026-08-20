package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net/http"
	"regexp"
	"time"

	"github.com/TBG-Chance/SentinelBox/internal/buildinfo"
)

type HealthChecker interface {
	Check(context.Context) (schemaVersion int, err error)
}

type Handler struct {
	logger        *slog.Logger
	health        HealthChecker
	build         buildinfo.Info
	healthTimeout time.Duration
	now           func() time.Time
}

type healthResponse struct {
	Status        string    `json:"status"`
	Service       string    `json:"service"`
	Database      string    `json:"database"`
	SchemaVersion int       `json:"schema_version"`
	Version       string    `json:"version"`
	Revision      string    `json:"revision"`
	BuildTime     string    `json:"build_time"`
	Time          time.Time `json:"time"`
	RequestID     string    `json:"request_id"`
}

func NewHandler(logger *slog.Logger, health HealthChecker, build buildinfo.Info, healthTimeout time.Duration) http.Handler {
	if logger == nil {
		logger = slog.Default()
	}
	handler := &Handler{
		logger:        logger,
		health:        health,
		build:         build,
		healthTimeout: healthTimeout,
		now:           time.Now,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/health", handler.healthCheck)
	return handler.requestID(handler.requestLog(handler.securityHeaders(mux)))
}

func (h *Handler) healthCheck(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), h.healthTimeout)
	defer cancel()

	statusCode := http.StatusOK
	response := healthResponse{
		Status:    "healthy",
		Service:   "sentinelbox",
		Database:  "ready",
		Version:   h.build.Version,
		Revision:  h.build.Revision,
		BuildTime: h.build.BuildTime,
		Time:      h.now().UTC(),
		RequestID: requestIDFromContext(r.Context()),
	}

	if h.health == nil {
		statusCode = http.StatusServiceUnavailable
		response.Status = "degraded"
		response.Database = "unavailable"
		h.logger.ErrorContext(r.Context(), "health check has no database provider")
	} else {
		version, err := h.health.Check(ctx)
		response.SchemaVersion = version
		if err != nil {
			statusCode = http.StatusServiceUnavailable
			response.Status = "degraded"
			response.Database = "unavailable"
			h.logger.ErrorContext(r.Context(), "database health check failed", "error", err)
		}
	}

	writeJSON(w, statusCode, response)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

type requestIDContextKey struct{}

var validRequestID = regexp.MustCompile(`^[A-Za-z0-9._-]{1,64}$`)

func (h *Handler) requestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-ID")
		if !validRequestID.MatchString(id) {
			id = newRequestID()
		}
		w.Header().Set("X-Request-ID", id)
		ctx := context.WithValue(r.Context(), requestIDContextKey{}, id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func newRequestID() string {
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		return hex.EncodeToString([]byte(time.Now().UTC().Format(time.RFC3339Nano)))
	}
	return hex.EncodeToString(value)
}

func requestIDFromContext(ctx context.Context) string {
	value, _ := ctx.Value(requestIDContextKey{}).(string)
	return value
}

func (h *Handler) securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(w, r)
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (r *statusRecorder) Unwrap() http.ResponseWriter {
	return r.ResponseWriter
}

func (r *statusRecorder) WriteHeader(status int) {
	if r.wroteHeader {
		return
	}
	r.wroteHeader = true
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func (h *Handler) requestLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		recorder := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(recorder, r)
		h.logger.InfoContext(
			r.Context(),
			"HTTP request completed",
			"method", r.Method,
			"path", r.URL.Path,
			"status", recorder.status,
			"duration_ms", time.Since(started).Milliseconds(),
			"request_id", requestIDFromContext(r.Context()),
		)
	})
}
