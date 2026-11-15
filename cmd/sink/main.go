// Command sink records task firings and reports peak concurrency.
package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Starovoitov/phasefleet/internal/sink"
)

func main() {
	listen := flag.String("listen", ":8080", "HTTP listen address")
	d := flag.Float64("d", 1, "task duration in seconds")
	flag.Parse()

	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, nil)))
	if *d <= 0 {
		slog.Error("d must be positive")
		os.Exit(2)
	}

	record := sink.New(time.Duration(*d * float64(time.Second)))
	srv := &http.Server{
		Addr:              *listen,
		Handler:           sink.Handler(record),
		ReadHeaderTimeout: 5 * time.Second,
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			slog.Error("shutdown", "err", err)
		}
	}()

	slog.Info("phasefleet sink", "listen", *listen, "d", *d)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		slog.Error("listen", "err", err)
		os.Exit(1)
	}
}
