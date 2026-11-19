// The running server's local socket for commands (backup).

package server

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// controlSocketName is the socket in DATA_DIR: only the server's own user (the container) reaches it.
const controlSocketName = "control.sock"

// ControlSocketPath is the socket of the server with this data directory.
func ControlSocketPath(dataDir string) string { return filepath.Join(dataDir, controlSocketName) }

// BackupFunc writes a backup into target, reporting progress lines; it returns a summary.
type BackupFunc func(ctx context.Context, target string, progress func(string)) (string, error)

// Control answers commands on the socket.
type Control struct {
	listener net.Listener
	path     string
	logger   *slog.Logger
	backup   BackupFunc
	running  atomic.Bool // one backup at a time

	ctx         context.Context
	cancel      context.CancelFunc
	connections sync.WaitGroup
}

// Protocol: one request line, then "progress <text>" lines and one "ok <summary>" or "error <message>".
const (
	requestLimit   = 4096
	requestTimeout = 10 * time.Second
)

// ListenControl opens the socket of dataDir, replacing a stale one; the database lock keeps a second server out.
func ListenControl(dataDir string, logger *slog.Logger, backup BackupFunc) (*Control, error) {
	path := ControlSocketPath(dataDir)

	if info, err := os.Lstat(path); err == nil {
		if info.Mode().Type() != fs.ModeSocket {
			return nil, fmt.Errorf("%s exists and is not a socket", path)
		}

		if err := os.Remove(path); err != nil {
			return nil, err
		}
	}

	// Made in a directory only this user can enter and moved out once it is 0600,
	// so nobody else can connect while it has the umask's mode.
	privateDir, err := os.MkdirTemp(dataDir, "control.")

	if err != nil {
		return nil, err
	}

	defer func() { _ = os.RemoveAll(privateDir) }()

	socketPath := filepath.Join(privateDir, "sock") // short: socket paths are limited to about 100 bytes
	listener, err := net.Listen("unix", socketPath)

	if err != nil {
		return nil, err
	}

	listener.(*net.UnixListener).SetUnlinkOnClose(false)

	if err := errors.Join(os.Chmod(socketPath, 0o600), os.Rename(socketPath, path)); err != nil {
		_ = listener.Close()

		return nil, err
	}

	ctx, cancel := context.WithCancel(context.Background())
	control := &Control{listener: listener, path: path, logger: logger, backup: backup, ctx: ctx, cancel: cancel}

	control.connections.Go(control.serve)

	return control, nil
}

// Close stops listening, cancels a running backup, waits for it and removes the socket.
func (control *Control) Close() error {
	control.cancel()
	err := control.listener.Close()
	control.connections.Wait()

	if removeErr := os.Remove(control.path); !errors.Is(removeErr, fs.ErrNotExist) {
		err = errors.Join(err, removeErr)
	}

	return err
}

func (control *Control) serve() {
	for {
		conn, err := control.listener.Accept()

		if err != nil {
			if control.ctx.Err() == nil {
				control.logger.Warn("control socket", "err", err)
			}

			return
		}

		control.connections.Go(func() {
			defer func() { _ = conn.Close() }()

			control.handle(conn)
		})
	}
}

func (control *Control) handle(conn net.Conn) {
	// Close ends every connection.
	stop := context.AfterFunc(control.ctx, func() { _ = conn.Close() })

	defer stop()

	_ = conn.SetReadDeadline(time.Now().Add(requestTimeout))
	line, err := bufio.NewReader(io.LimitReader(conn, requestLimit)).ReadString('\n')

	if err != nil {
		return
	}

	command, target, _ := strings.Cut(strings.TrimSpace(line), " ")

	switch {
	case command != "backup":
		reply(conn, "error", fmt.Sprintf("unknown command %q", command))

		return
	case !control.running.CompareAndSwap(false, true):
		reply(conn, "error", "another backup is running")

		return
	}

	defer control.running.Store(false)

	ctx, cancel := context.WithCancel(control.ctx)

	defer cancel()

	// The client going away (Ctrl+C) cancels the backup.
	_ = conn.SetReadDeadline(time.Time{})

	go func() {
		_, _ = conn.Read(make([]byte, 1))

		cancel()
	}()

	summary, err := control.backup(ctx, target, func(text string) {
		if !reply(conn, "progress", text) {
			cancel()
		}
	})

	if err != nil {
		reply(conn, "error", err.Error())

		return
	}

	reply(conn, "ok", summary)
}

// reply writes one line; false when the client is gone.
func reply(conn net.Conn, kind, text string) bool {
	_, err := fmt.Fprintf(conn, "%s %s\n", kind, strings.ReplaceAll(text, "\n", " "))

	return err == nil
}

// ErrNoServer: no server answers on the socket.
var ErrNoServer = errors.New("no server answers on the control socket")

// RequestBackup asks the server of dataDir to write a backup into target; progress lines go to out.
func RequestBackup(ctx context.Context, dataDir, target string, out io.Writer) (string, error) {
	var dialer net.Dialer

	dialCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	conn, err := dialer.DialContext(dialCtx, "unix", ControlSocketPath(dataDir))

	cancel()

	if err != nil {
		return "", fmt.Errorf("%w: %w", ErrNoServer, err)
	}

	defer func() { _ = conn.Close() }()

	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })

	defer stop()

	if _, err := fmt.Fprintf(conn, "backup %s\n", target); err != nil {
		return "", err
	}

	scanner := bufio.NewScanner(conn)

	for scanner.Scan() {
		kind, text, _ := strings.Cut(scanner.Text(), " ")

		switch kind {
		case "progress":
			_, _ = fmt.Fprintln(out, text)
		case "ok":
			return text, nil
		case "error":
			return "", errors.New(text)
		}
	}

	if ctx.Err() != nil {
		return "", ctx.Err()
	}

	return "", fmt.Errorf("the server closed the connection before the backup finished: %w", errors.Join(scanner.Err(), io.ErrUnexpectedEOF))
}
