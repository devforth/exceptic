package app

import (
	"log/slog"

	"github.com/devforth/exceptic/internal/notify"
	"github.com/devforth/exceptic/internal/platform/background"
	"github.com/devforth/exceptic/internal/platform/config"
	"github.com/devforth/exceptic/internal/storage/database"
)

// lanes is every periodic job of the server (schedule: config/schedule.go;
// the database lane: database.Store.Lane).
// Besides them, the ingest pipeline, the notifier's dispatcher and workers and
// the HTTP server run as long as the server does.
func lanes(db *database.Store, notifier *notify.Notifier, maxBytes int64, logger *slog.Logger) []background.Lane {
	return []background.Lane{
		db.Lane(maxBytes, logger),
		{Name: "notifications", Jobs: []background.Job{
			{Name: "outbox cleanup", First: config.OutboxCleanupDelay, Every: config.OutboxCleanupInterval, Timeout: config.OutboxCleanupTimeout,
				Run: notifier.CleanOutbox},
		}},
	}
}
