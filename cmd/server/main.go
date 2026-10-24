// Command server runs the Streaming Availability Service.
package main

import (
	"log/slog"
	"os"

	"github.com/pjserol/stan-availability-service/internal/api"
	"github.com/pjserol/stan-availability-service/internal/config"
	"github.com/pjserol/stan-availability-service/internal/store"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(logger); err != nil {
		logger.Error("server exited", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	srv := api.NewServer(store.New(), logger)
	_ = srv
	_ = cfg

	// TODO: ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, os.Interrupt)
	// TODO: http.Server{Addr: ":"+cfg.Port, Handler: srv.Routes(), Read/Write/IdleTimeouts}
	// TODO: ListenAndServe in a goroutine; ignore http.ErrServerClosed
	// TODO: on <-ctx.Done(): shutdown ctx with cfg.ShutdownTimeout, httpSrv.Shutdown(shCtx),
	//       wrap errors with %w, log drain start/finish
	return nil
}
