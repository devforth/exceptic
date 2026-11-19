package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devforth/exceptic/internal/storage/database"
)

// An unknown command prints the usage instead of starting the server.
func TestUnknownCommand(t *testing.T) {
	// Should the server start after all, it fails at once on this data directory.
	file := filepath.Join(t.TempDir(), "file")

	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("DATA_DIR", filepath.Join(file, "data"))

	for _, args := range [][]string{{"reset-pasword"}, {"--help"}} {
		if code := execute(args); code != 2 {
			t.Errorf("%v: exit code %d, want 2", args, code)
		}
	}
}

// "Stop the server first" only when a server holds the database.
func TestResetPasswordOpenErrors(t *testing.T) {
	empty := t.TempDir()
	t.Setenv("DATA_DIR", empty)

	if err := resetPassword(nil); err == nil || !strings.Contains(err.Error(), "no database in") {
		t.Errorf("without a database: %v", err)
	}

	if entries, _ := os.ReadDir(empty); len(entries) != 0 {
		t.Errorf("a database was created: %v", entries)
	}

	damaged := t.TempDir()
	t.Setenv("DATA_DIR", damaged)

	if err := os.WriteFile(database.Location(damaged), nil, 0o600); err != nil {
		t.Fatal(err)
	}

	if err := resetPassword(nil); err == nil || strings.Contains(err.Error(), "stop the server") {
		t.Errorf("damaged database: %v", err)
	}

	locked := t.TempDir()
	t.Setenv("DATA_DIR", locked)

	db, err := database.Open(locked)

	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = db.Close() }()

	if err := resetPassword(nil); err == nil || !strings.Contains(err.Error(), "stop the server first") {
		t.Errorf("database held by a server: %v", err)
	}
}
