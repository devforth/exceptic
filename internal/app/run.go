package app

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/devforth/exceptic/internal/platform/background"
	"github.com/devforth/exceptic/internal/platform/logging"
)

const shutdownTimeout = 10 * time.Second

// Run serves on listener until ctx ends, with the background work: the
// pipeline, the periodic jobs and the control socket. Everything stops in the
// reverse order; the store is the caller's.
func (app *App) Run(ctx context.Context, listener net.Listener) error {
	settings, logger, db := app.options.Settings, app.options.Logger, app.options.Store

	defer app.closeNotifier()

	jobs := lanes(db, app.notifier, settings.Retention.MaxDatabaseBytes, logger)

	logger.Info("background jobs", append([]any{logging.Block}, background.Describe(jobs...)...)...)

	defer background.Start(ctx, logger, jobs...)()

	// It stores what it holds before it stops, after the HTTP server.
	defer app.runPipeline()()

	if controlServer := listenControl(settings.DataDir, db, logger); controlServer != nil {
		defer func() { _ = controlServer.Close() }()
	}

	httpServer := &http.Server{
		Handler:           app.Handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	served := make(chan error, 1)

	go func() { served <- httpServer.Serve(listener) }()

	select {
	case err := <-served:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-ctx.Done():
		logger.Info("shutting down")

		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)

		defer cancel()

		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("shutdown: %w", err)
		}
	}

	return nil
}

// Start runs the pipeline of a server whose Handler the caller serves
// (tests); stop stores what it holds and sends the queued notifications.
func (app *App) Start() (stop func()) {
	stopPipeline := app.runPipeline()

	return func() {
		stopPipeline()
		app.closeNotifier()
	}
}

// WaitStored waits until every envelope accepted so far is stored (or dropped)
// and its notifications queued; sending them is asynchronous.
func (app *App) WaitStored(ctx context.Context) error {
	return app.pipeline.WaitStored(ctx)
}

// runPipeline stores accepted envelopes until stop, which returns once the
// pipeline has stored what it holds.
func (app *App) runPipeline() (stop func()) {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})

	go func() {
		defer close(done)

		app.pipeline.Run(ctx)
	}()

	return func() {
		cancel()
		<-done
	}
}

// closeNotifier sends what is still queued; it gives up after a while so shutdown cannot hang.
func (app *App) closeNotifier() {
	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)

	defer cancel()

	if err := app.notifier.Close(ctx); err != nil {
		app.options.Logger.Warn("notifications not sent before shutdown", "err", err)
	}
}
