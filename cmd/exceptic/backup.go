package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/devforth/exceptic/internal/platform/config"
	"github.com/devforth/exceptic/internal/server"
	"github.com/devforth/exceptic/internal/storage/database"
)

// backup is `exceptic backup [dir]`: a copy of the database, made by the running server or, when none runs, here.
func backup(args []string) error {
	if len(args) > 1 {
		return errors.New("usage: exceptic backup [absolute path of a new or empty directory; default DATA_DIR/backups/<time>]")
	}

	target := ""

	if len(args) == 1 {
		target = args[0]
	}

	settings, err := config.Load()

	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)

	defer stop()

	return runBackup(ctx, settings.DataDir, target, os.Stdout)
}

func runBackup(ctx context.Context, dataDir, target string, out io.Writer) error {
	if target == "" {
		base, err := filepath.Abs(filepath.Join(dataDir, "backups"))

		if err != nil {
			return err
		}

		target = filepath.Join(base, time.Now().UTC().Format("2006-01-02T150405Z"))
	}

	// The server resolves paths on its own, so a relative one would mean something else there.
	if !filepath.IsAbs(target) {
		return fmt.Errorf("the backup path %q is not absolute", target)
	}

	target = filepath.Clean(target)

	summary, err := server.RequestBackup(ctx, dataDir, target, out)

	if errors.Is(err, server.ErrNoServer) {
		summary, err = backupOffline(ctx, dataDir, target, out, err)
	}

	if err != nil {
		return err
	}

	_, _ = fmt.Fprintln(out, summary)

	return nil
}

// backupOffline opens the database itself; noServer says why the socket did not answer.
func backupOffline(ctx context.Context, dataDir, target string, out io.Writer, noServer error) (string, error) {
	if _, err := os.Stat(database.Location(dataDir)); err != nil {
		return "", fmt.Errorf("no database in %s: %w", dataDir, err)
	}

	_, _ = fmt.Fprintln(out, "no server answers, the database is read here")

	result, err := database.BackupStopped(ctx, dataDir, target, database.BackupProgress(func(text string) { _, _ = fmt.Fprintln(out, text) }))

	if errors.Is(err, database.ErrLocked) {
		return "", fmt.Errorf("the database is open in another process, but no server answers on %s (%w); stop that process and run the command again",
			server.ControlSocketPath(dataDir), noServer)
	}

	if err != nil {
		return "", err
	}

	return result.Summary(target), nil
}
