package leveldb

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/devforth/exceptic/internal/storage/kv"
	"github.com/devforth/exceptic/internal/storage/kv/enginetest"
)

func TestEngine(t *testing.T) {
	t.Run("on disk", func(t *testing.T) { enginetest.Run(t, Open, true) })
	t.Run("in memory", func(t *testing.T) {
		enginetest.Run(t, func(string) (kv.Engine, error) { return OpenMemory(), nil }, false)
	})
}

// The database directory is its owner's only, a new one and one made before.
func TestOpenRestrictsTheDirectory(t *testing.T) {
	existing := filepath.Join(t.TempDir(), "existing")

	if err := os.Mkdir(existing, 0o755); err != nil {
		t.Fatal(err)
	}

	for _, dir := range []string{filepath.Join(t.TempDir(), "data", "leveldb"), existing} {
		engine, err := Open(dir)

		if err != nil {
			t.Fatal(err)
		}

		_ = engine.Close()
		info, err := os.Stat(dir)

		if err != nil {
			t.Fatal(err)
		}

		if info.Mode().Perm() != 0o700 {
			t.Errorf("%s: mode %v, want 0700", dir, info.Mode().Perm())
		}
	}
}

func TestOpenLocked(t *testing.T) {
	dir := t.TempDir()
	engine, err := Open(dir)

	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = engine.Close() }()

	if _, err := Open(dir); !errors.Is(err, kv.ErrLocked) {
		t.Fatalf("second open: %v, want kv.ErrLocked", err)
	}
}
