package main

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func newTestServer() http.Handler {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	s := &server{
		version:   "1.2.3",
		commit:    "abc1234",
		buildDate: "2026-01-01T00:00:00Z",
		env:       "test",
		hostname:  "test-host",
		startedAt: start,
		now:       func() time.Time { return start.Add(90 * time.Second) },
		logger:    slog.New(slog.NewJSONHandler(io.Discard, nil)),
	}
	return s.handler()
}

func do(t *testing.T, h http.Handler, method, path string, hdr map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func decode(t *testing.T, rec *httptest.ResponseRecorder) map[string]string {
	t.Helper()
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Fatalf("Content-Type = %q, want application/json", ct)
	}
	var m map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		t.Fatalf("invalid JSON %q: %v", rec.Body.String(), err)
	}
	return m
}

func TestRoot(t *testing.T) {
	rec := do(t, newTestServer(), http.MethodGet, "/", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	m := decode(t, rec)
	want := map[string]string{
		"app": "go-lab", "version": "1.2.3", "commit": "abc1234",
		"build_date": "2026-01-01T00:00:00Z", "hostname": "test-host",
		"env": "test", "uptime": "1m30s",
	}
	for k, v := range want {
		if m[k] != v {
			t.Errorf("%s = %q, want %q", k, m[k], v)
		}
	}
	if !strings.HasPrefix(m["go_version"], "go") {
		t.Errorf("go_version = %q", m["go_version"])
	}
}

func TestHealthz(t *testing.T) {
	rec := do(t, newTestServer(), http.MethodGet, "/healthz", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if m := decode(t, rec); m["status"] != "ok" {
		t.Errorf("body = %v", m)
	}
}

func TestVersion(t *testing.T) {
	rec := do(t, newTestServer(), http.MethodGet, "/version", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	m := decode(t, rec)
	if len(m) != 3 || m["version"] != "1.2.3" || m["commit"] != "abc1234" || m["build_date"] != "2026-01-01T00:00:00Z" {
		t.Errorf("body = %v", m)
	}
}

func TestNotFound(t *testing.T) {
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		rec := do(t, newTestServer(), method, "/nope", nil)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("%s status = %d", method, rec.Code)
		}
		if m := decode(t, rec); m["error"] != "not_found" || m["message"] == "" {
			t.Errorf("body = %v", m)
		}
	}
}

func TestMethodNotAllowed(t *testing.T) {
	for _, path := range []string{"/", "/healthz", "/version"} {
		rec := do(t, newTestServer(), http.MethodPost, path, nil)
		if rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("%s status = %d", path, rec.Code)
		}
		if got := rec.Header().Get("Allow"); got != http.MethodGet {
			t.Errorf("%s Allow = %q", path, got)
		}
		if m := decode(t, rec); m["error"] != "method_not_allowed" {
			t.Errorf("body = %v", m)
		}
	}
}

func TestRequestID(t *testing.T) {
	h := newTestServer()

	generated := do(t, h, http.MethodGet, "/healthz", nil).Header().Get(requestIDHeader)
	if len(generated) != 32 {
		t.Errorf("generated id = %q, want 32 hex chars", generated)
	}
	other := do(t, h, http.MethodGet, "/healthz", nil).Header().Get(requestIDHeader)
	if generated == other {
		t.Error("generated ids should differ between requests")
	}

	reused := do(t, h, http.MethodGet, "/healthz", map[string]string{requestIDHeader: "client-id-42"})
	if got := reused.Header().Get(requestIDHeader); got != "client-id-42" {
		t.Errorf("reused id = %q", got)
	}

	tooLong := strings.Repeat("a", maxRequestIDLen+1)
	replaced := do(t, h, http.MethodGet, "/healthz", map[string]string{requestIDHeader: tooLong})
	if got := replaced.Header().Get(requestIDHeader); got == tooLong || got == "" {
		t.Errorf("oversized id should be replaced, got %q", got)
	}
}

func TestLoadConfig(t *testing.T) {
	env := func(m map[string]string) func(string) string {
		return func(k string) string { return m[k] }
	}

	t.Run("defaults", func(t *testing.T) {
		cfg, err := loadConfig(env(nil))
		if err != nil {
			t.Fatal(err)
		}
		if cfg.Port != 8080 || cfg.Env != "dev" || cfg.LogLevel != slog.LevelInfo {
			t.Errorf("cfg = %+v", cfg)
		}
	})

	t.Run("custom", func(t *testing.T) {
		cfg, err := loadConfig(env(map[string]string{"PORT": "9090", "APP_ENV": "prod", "LOG_LEVEL": "debug"}))
		if err != nil {
			t.Fatal(err)
		}
		if cfg.Port != 9090 || cfg.Env != "prod" || cfg.LogLevel != slog.LevelDebug {
			t.Errorf("cfg = %+v", cfg)
		}
	})

	invalid := []map[string]string{
		{"PORT": "abc"},
		{"PORT": "0"},
		{"PORT": "70000"},
		{"PORT": "-1"},
		{"LOG_LEVEL": "verbose"},
		{"LOG_LEVEL": "INFO+2"},
	}
	for _, m := range invalid {
		if _, err := loadConfig(env(m)); err == nil {
			t.Errorf("loadConfig(%v) should fail", m)
		}
	}
}
