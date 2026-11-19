package app

import (
	"context"
	"log/slog"

	"github.com/devforth/exceptic/internal/platform/config"
	"github.com/devforth/exceptic/internal/server"
	"github.com/devforth/exceptic/internal/storage/database"
)

// listenControl opens the server's control socket; without it the server still runs, and backups need it stopped.
func listenControl(dataDir string, db *database.Store, logger *slog.Logger) *server.Control {
	socket, err := server.ListenControl(dataDir, logger, BackupHandler(db, logger))

	if err != nil {
		logger.Warn("control socket not available, the backup command needs the server stopped", "err", err)

		return nil
	}

	return socket
}

// BackupHandler writes the backups the control socket asks for.
func BackupHandler(db *database.Store, logger *slog.Logger) server.BackupFunc {
	return func(ctx context.Context, target string, progress func(string)) (string, error) {
		logger.Info("backup started", "target", target)

		result, err := db.Backup(ctx, target, database.BackupProgress(progress))

		if err != nil {
			logger.Warn("backup failed", "target", target, "err", err)

			return "", err
		}

		logger.Info("backup written", "target", target, "keys", result.Keys, "size", config.FormatSize(result.Size))

		return result.Summary(target), nil
	}
}
