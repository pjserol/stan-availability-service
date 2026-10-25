// Command server runs the Streaming Availability Service.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/pjserol/stan-availability-service/internal/api"
	"github.com/pjserol/stan-availability-service/internal/config"
	"github.com/pjserol/stan-availability-service/internal/store"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, os.Interrupt)
	err := run(ctx, logger)
	stop()
	if err != nil {
		logger.Error("server exited", slog.Any("error", err))
		os.Exit(1)
	}
}

func run(ctx context.Context, logger *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	ln, err := net.Listen("tcp", ":"+cfg.Port)
	if err != nil {
		return fmt.Errorf("listen on port %s: %w", cfg.Port, err)
	}
	handler := api.NewServer(store.New(), logger).Routes()
	logger.Info("server started", slog.String("addr", ln.Addr().String()),
		slog.String("shutdown_timeout", cfg.ShutdownTimeout.String()))
	return serve(ctx, ln, handler, cfg.ShutdownTimeout, logger)
}

// serve handles requests on ln until ctx is cancelled, then stops accepting
// connections and waits up to drainTimeout for in-flight requests to finish.
// Request contexts are not tied to ctx, so a signal drains rather than aborts.
func serve(ctx context.Context, ln net.Listener, h http.Handler, drainTimeout time.Duration, logger *slog.Logger) error {
	srv := &http.Server{
		Handler:           h,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
		ErrorLog:          slog.NewLogLogger(logger.Handler(), slog.LevelError),
	}
	serveErr := make(chan error, 1)
	go func() { serveErr <- srv.Serve(ln) }()

	select {
	case err := <-serveErr:
		return fmt.Errorf("serve: %w", err)
	case <-ctx.Done():
	}

	logger.Info("shutdown started", slog.String("drain_timeout", drainTimeout.String()))
	// ctx is already cancelled; keep its values but give the drain its own deadline.
	drainCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), drainTimeout)
	defer cancel()
	if err := srv.Shutdown(drainCtx); err != nil {
		return errors.Join(fmt.Errorf("drain in-flight requests: %w", err), srv.Close())
	}
	if err := <-serveErr; !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("serve: %w", err)
	}
	logger.Info("shutdown complete")
	return nil
}
