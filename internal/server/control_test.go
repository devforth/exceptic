package server

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func listen(t *testing.T, dataDir string, backup BackupFunc) *Control {
	t.Helper()

	server, err := ListenControl(dataDir, slog.New(slog.NewTextHandler(io.Discard, nil)), backup)

	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = server.Close() })

	return server
}

func TestBackupThroughSocket(t *testing.T) {
	dataDir := t.TempDir()
	received := make(chan string, 1)

	listen(t, dataDir, func(_ context.Context, target string, progress func(string)) (string, error) {
		received <- target

		progress("copied 1 MB")
		progress("copied 2 MB")

		return "written to " + target, nil
	})

	var out bytes.Buffer

	summary, err := RequestBackup(context.Background(), dataDir, "/backups/first", &out)

	if err != nil || summary != "written to /backups/first" {
		t.Errorf("summary %q, error %v", summary, err)
	}

	if target := <-received; target != "/backups/first" {
		t.Errorf("server got %q", target)
	}

	if out.String() != "copied 1 MB\ncopied 2 MB\n" {
		t.Errorf("progress %q", out.String())
	}

	info, err := os.Stat(ControlSocketPath(dataDir))

	if err != nil || info.Mode().Perm() != 0o600 {
		t.Errorf("socket mode %v %v, want 0600", info.Mode(), err)
	}

	// The directory it was made in is gone.
	if entries, err := os.ReadDir(dataDir); err != nil || len(entries) != 1 {
		t.Errorf("data directory holds %v (%v), want only the socket", entries, err)
	}
}

func TestBackupErrors(t *testing.T) {
	dataDir := t.TempDir()

	listen(t, dataDir, func(context.Context, string, func(string)) (string, error) {
		return "", errors.New("disk full")
	})

	if _, err := RequestBackup(context.Background(), dataDir, "/backups/first", io.Discard); err == nil || err.Error() != "disk full" {
		t.Errorf("failed backup: %v", err)
	}

	conn, err := net.Dial("unix", ControlSocketPath(dataDir))

	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = conn.Close() }()

	_, _ = conn.Write([]byte("delete /data\n"))
	answer, _ := io.ReadAll(conn)

	if !strings.HasPrefix(string(answer), "error unknown command") {
		t.Errorf("unknown command answered %q", answer)
	}
}

func TestNoServer(t *testing.T) {
	dataDir := t.TempDir()

	if _, err := RequestBackup(context.Background(), dataDir, "/backups/first", io.Discard); !errors.Is(err, ErrNoServer) {
		t.Errorf("without a server: %v", err)
	}

	// A socket left by a killed server: nobody answers, and the next start replaces it.
	stale, err := net.Listen("unix", ControlSocketPath(dataDir))

	if err != nil {
		t.Fatal(err)
	}

	stale.(*net.UnixListener).SetUnlinkOnClose(false)
	_ = stale.Close()

	if _, err := RequestBackup(context.Background(), dataDir, "/backups/first", io.Discard); !errors.Is(err, ErrNoServer) {
		t.Errorf("stale socket: %v", err)
	}

	server := listen(t, dataDir, func(context.Context, string, func(string)) (string, error) { return "done", nil })

	if summary, err := RequestBackup(context.Background(), dataDir, "/backups/first", io.Discard); err != nil || summary != "done" {
		t.Errorf("after replacing the stale socket: %q %v", summary, err)
	}

	if err := server.Close(); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(ControlSocketPath(dataDir)); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("socket after Close: %v", err)
	}
}

// A client that goes away (Ctrl+C) cancels the backup.
func TestClientGoneCancels(t *testing.T) {
	dataDir := t.TempDir()
	started, canceled := make(chan struct{}), make(chan struct{})

	listen(t, dataDir, func(ctx context.Context, _ string, _ func(string)) (string, error) {
		close(started)
		<-ctx.Done()
		close(canceled)

		return "", ctx.Err()
	})

	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)

	go func() {
		_, err := RequestBackup(ctx, dataDir, "/backups/first", io.Discard)

		result <- err
	}()

	<-started
	cancel()

	select {
	case <-canceled:
	case <-time.After(5 * time.Second):
		t.Fatal("the backup went on after the client left")
	}

	if err := <-result; !errors.Is(err, context.Canceled) {
		t.Errorf("client: %v", err)
	}
}

// One backup at a time; Close cancels a running one and waits for it.
func TestOneBackupAndClose(t *testing.T) {
	dataDir := t.TempDir()
	started := make(chan struct{})

	var finished atomic.Bool

	server := listen(t, dataDir, func(ctx context.Context, _ string, _ func(string)) (string, error) {
		close(started)
		<-ctx.Done()
		time.Sleep(50 * time.Millisecond)
		finished.Store(true)

		return "", ctx.Err()
	})

	first := make(chan error, 1)

	go func() {
		_, err := RequestBackup(context.Background(), dataDir, "/backups/first", io.Discard)

		first <- err
	}()

	<-started

	if _, err := RequestBackup(context.Background(), dataDir, "/backups/second", io.Discard); err == nil || err.Error() != "another backup is running" {
		t.Errorf("second backup: %v", err)
	}

	if err := server.Close(); err != nil {
		t.Fatal(err)
	}

	if !finished.Load() {
		t.Error("Close returned before the backup stopped")
	}

	if err := <-first; err == nil {
		t.Error("the canceled backup succeeded")
	}
}
