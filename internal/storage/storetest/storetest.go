// Package storetest opens a store for tests.
package storetest

import (
	"context"
	"testing"
	"time"

	"github.com/devforth/exceptic/internal/platform/config"
	"github.com/devforth/exceptic/internal/storage/database"
	"github.com/devforth/exceptic/internal/storage/store"
)

// Open opens a new store in a temporary directory, closed with the test.
func Open(t testing.TB) *database.Store {
	t.Helper()

	opened, err := database.Open(t.TempDir())

	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = opened.Close() })

	return opened
}

// AddProjects stores projects with their own ids and keys, such as
// fixtures.Projects (the DSNs the fixtures were recorded with), before the
// registry opens; those stored already stay as they are.
func AddProjects(t testing.TB, opened *database.Store, projects config.Projects) {
	t.Helper()

	records := make([]store.Project, 0, len(projects))

	for _, project := range projects {
		records = append(records, store.Project{
			ID: project.ID, Name: project.Name, PublicKey: project.PublicKey, Color: project.Color, LogLevels: project.LogLevels,
			DiskShare: project.DiskShare, AllowedOrigins: project.AllowedOrigins, Paused: project.Paused,
		})
	}

	if err := opened.AddProjects(context.Background(), time.Now(), records...); err != nil {
		t.Fatal(err)
	}
}
