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

	"github.com/zumlabs/zums-proxy-relay/internal/proxy"
)

type config struct {
	host     string
	port     int
	authUser string
	authPass string
	debug    bool
}

func main() {
	if err := run(context.Background(), os.Getenv, os.Stdout); err != nil {
		slog.Error("server stopped", "err", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, getenv func(string) string, stdout io.Writer) error {
	cfg, err := loadConfig(getenv)
	if err != nil {
		return err
	}

	level := slog.LevelInfo
	if cfg.debug {
		level = slog.LevelDebug
	}
	logger := slog.New(slog.NewJSONHandler(stdout, &slog.HandlerOptions{Level: level}))
	addr := net.JoinHostPort(cfg.host, strconv.Itoa(cfg.port))
	server := &http.Server{
		Addr:              addr,
		Handler:           proxy.New(logger, cfg.authUser, cfg.authPass),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		logger.Info("relay listening", "addr", addr, "auth", "required")
		errCh <- server.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		logger.Info("shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("shutdown server: %w", err)
		}
		return nil
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("listen and serve: %w", err)
	}
}

func loadConfig(getenv func(string) string) (config, error) {
	cfg := config{
		host:     valueOr(getenv("HOST"), "0.0.0.0"),
		authUser: getenv("AUTH_USER"),
		authPass: getenv("AUTH_PASS"),
		debug:    getenv("DEBUG") == "true",
	}
	portString := valueOr(getenv("PORT"), "8080")
	port, err := strconv.Atoi(portString)
	if err != nil || port < 1 || port > 65535 {
		return config{}, fmt.Errorf("invalid port %q", portString)
	}
	cfg.port = port
	if cfg.authUser == "" || cfg.authPass == "" {
		return config{}, errors.New("auth_user and auth_pass are required; refusing to start an open proxy")
	}
	return cfg, nil
}

func valueOr(value, fallback string) string {
	if value != "" {
		return value
	}
	return fallback
}
