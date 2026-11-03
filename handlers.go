package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net/http"
	"runtime"
	"time"
)

const (
	appName         = "go-lab"
	requestIDHeader = "X-Request-ID"
	maxRequestIDLen = 128
)

// Build metadata. Overridden at link time, for example:
//
//	go build -ldflags "-X main.version=1.0.0 -X main.commit=abc1234 -X main.buildDate=2026-01-01T00:00:00Z"
var (
	version   = "dev"
	commit    = "unknown"
	buildDate = "unknown"
)

// server holds everything the handlers need, so they can be tested without
// touching globals, the network or the environment.
type server struct {
	version   string
	commit    string
	buildDate string
	env       string
	hostname  string
	startedAt time.Time
	now       func() time.Time
	logger    *slog.Logger
}

// route describes a path and the single method it accepts.
type route struct {
	method  string
	handler http.HandlerFunc
}

// handler returns the full HTTP handler: routing wrapped by the request ID
// and access log middleware.
func (s *server) handler() http.Handler {
	routes := map[string]route{
		"/":        {http.MethodGet, s.handleRoot},
		"/healthz": {http.MethodGet, s.handleHealthz},
		"/version": {http.MethodGet, s.handleVersion},
	}

	router := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rt, ok := routes[r.URL.Path]
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{
				"error":   "not_found",
				"message": "no route matches " + r.URL.Path,
			})
			return
		}
		if r.Method != rt.method {
			w.Header().Set("Allow", rt.method)
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{
				"error":   "method_not_allowed",
				"message": r.Method + " is not allowed on " + r.URL.Path + "; allowed: " + rt.method,
			})
			return
		}
		rt.handler(w, r)
	})

	return s.withRequestID(s.withLogging(router))
}

func (s *server) handleRoot(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"app":        appName,
		"version":    s.version,
		"commit":     s.commit,
		"build_date": s.buildDate,
		"hostname":   s.hostname,
		"go_version": runtime.Version(),
		"env":        s.env,
		"uptime":     s.now().Sub(s.startedAt).Round(time.Second).String(),
	})
}

func (s *server) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *server) handleVersion(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"version":    s.version,
		"commit":     s.commit,
		"build_date": s.buildDate,
	})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// withRequestID reuses a sane incoming X-Request-ID or generates a new one,
// and always echoes it in the response.
func (s *server) withRequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get(requestIDHeader)
		if !validRequestID(id) {
			id = newRequestID()
			r.Header.Set(requestIDHeader, id)
		}
		w.Header().Set(requestIDHeader, id)
		next.ServeHTTP(w, r)
	})
}

// validRequestID accepts non-empty printable ASCII up to maxRequestIDLen,
// which keeps untrusted values safe to log and to echo in a header.
func validRequestID(id string) bool {
	if id == "" || len(id) > maxRequestIDLen {
		return false
	}
	for i := 0; i < len(id); i++ {
		if id[i] < 0x21 || id[i] > 0x7e {
			return false
		}
	}
	return true
}

func newRequestID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "unknown"
	}
	return hex.EncodeToString(b)
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

// withLogging emits one structured log record per request.
func (s *server) withLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		s.logger.Info("request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", rec.status,
			"duration_ms", float64(time.Since(start).Microseconds())/1000,
			"request_id", r.Header.Get(requestIDHeader),
		)
	})
}
