package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"
)

const (
	// healthcheckArg switches the binary into probe mode (used by the Docker
	// HEALTHCHECK, since the final image has no curl or shell).
	healthcheckArg = "healthcheck"

	// Docker waits 10s by default before SIGKILL; stay well below it.
	shutdownTimeout = 5 * time.Second

	readHeaderTimeout = 5 * time.Second
	readTimeout       = 10 * time.Second
	writeTimeout      = 10 * time.Second
	idleTimeout       = 60 * time.Second
	maxHeaderBytes    = 16 << 10 // 16 KiB

	healthcheckTimeout = 2 * time.Second
)

// config holds the validated runtime configuration.
type config struct {
	Port     int
	Env      string
	LogLevel slog.Level
}

// loadConfig reads PORT, APP_ENV and LOG_LEVEL through lookup (os.Getenv in
// production), applying defaults and rejecting invalid values.
func loadConfig(lookup func(string) string) (config, error) {
	get := func(key, def string) string {
		if v := lookup(key); v != "" {
			return v
		}
		return def
	}

	cfg := config{Env: get("APP_ENV", "dev")}

	portStr := get("PORT", "8080")
	port, err := strconv.Atoi(portStr)
	if err != nil || port < 1 || port > 65535 {
		return config{}, fmt.Errorf("invalid PORT %q: must be an integer between 1 and 65535", portStr)
	}
	cfg.Port = port

	levelStr := get("LOG_LEVEL", "info")
	if err := cfg.LogLevel.UnmarshalText([]byte(levelStr)); err != nil || !validLevel(levelStr) {
		return config{}, fmt.Errorf("invalid LOG_LEVEL %q: must be one of debug, info, warn, error", levelStr)
	}
	return cfg, nil
}

// validLevel restricts LOG_LEVEL to the four documented names (slog would
// also accept offsets such as "INFO+2").
func validLevel(s string) bool {
	switch s {
	case "debug", "info", "warn", "error", "DEBUG", "INFO", "WARN", "ERROR":
		return true
	}
	return false
}

func main() {
	os.Exit(run(os.Args[1:], os.Getenv, os.Stdout, os.Stderr))
}

func run(args []string, getenv func(string) string, stdout, stderr io.Writer) int {
	cfg, err := loadConfig(getenv)
	if err != nil {
		fmt.Fprintln(stderr, "configuration error:", err)
		return 2
	}

	if len(args) > 0 {
		if args[0] != healthcheckArg {
			fmt.Fprintf(stderr, "unknown argument %q (supported: %q)\n", args[0], healthcheckArg)
			return 2
		}
		if err := probe(cfg.Port); err != nil {
			fmt.Fprintln(stderr, "healthcheck failed:", err)
			return 1
		}
		return 0
	}

	logger := slog.New(slog.NewJSONHandler(stdout, &slog.HandlerOptions{Level: cfg.LogLevel}))
	hostname, _ := os.Hostname()
	app := &server{
		version:   version,
		commit:    commit,
		buildDate: buildDate,
		env:       cfg.Env,
		hostname:  hostname,
		startedAt: time.Now(),
		now:       time.Now,
		logger:    logger,
	}

	srv := &http.Server{
		Addr:              net.JoinHostPort("0.0.0.0", strconv.Itoa(cfg.Port)),
		Handler:           app.handler(),
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       readTimeout,
		WriteTimeout:      writeTimeout,
		IdleTimeout:       idleTimeout,
		MaxHeaderBytes:    maxHeaderBytes,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	// Bind first so a port conflict fails fast with a clear error.
	ln, err := net.Listen("tcp", srv.Addr)
	if err != nil {
		logger.Error("listen failed", "error", err.Error())
		return 1
	}

	logger.Info("starting", "version", version, "commit", commit, "port", cfg.Port, "env", cfg.Env)

	serveErr := make(chan error, 1)
	go func() { serveErr <- srv.Serve(ln) }()

	select {
	case err := <-serveErr:
		logger.Error("server failed", "error", err.Error())
		return 1
	case <-ctx.Done():
	}
	stop() // restore default signal behaviour: a second signal kills us

	logger.Info("shutting down", "timeout", shutdownTimeout.String())
	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	// Shutdown closes the listener and idle keep-alive connections right
	// away, then waits for in-flight requests until the deadline.
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("graceful shutdown timed out, forcing close", "error", err.Error())
		_ = srv.Close()
		return 1
	}
	logger.Info("shutdown complete")
	return 0
}

// probe requests /healthz on the local port and succeeds only on HTTP 200.
func probe(port int) error {
	url := "http://" + net.JoinHostPort("127.0.0.1", strconv.Itoa(port)) + "/healthz"
	client := &http.Client{Timeout: healthcheckTimeout}
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode != http.StatusOK {
		return errors.New("unexpected status " + resp.Status)
	}
	return nil
}
