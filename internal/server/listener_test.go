package server

import (
	"errors"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Connections over the limit wait in the accept queue until one closes; none is refused.
func TestLimitListener(t *testing.T) {
	inner, err := net.Listen("tcp", "127.0.0.1:0")

	if err != nil {
		t.Fatal(err)
	}

	var full atomic.Int32

	listener := LimitListener(inner, 3, func() { full.Add(1) })

	defer func() { _ = listener.Close() }()

	var (
		open, most atomic.Int32
		served     sync.WaitGroup
	)

	go func() {
		for {
			conn, err := listener.Accept()

			if err != nil {
				return
			}

			go func() {
				now := open.Add(1)

				for previous := most.Load(); now > previous && !most.CompareAndSwap(previous, now); previous = most.Load() {
				}

				time.Sleep(20 * time.Millisecond)
				open.Add(-1)

				_, _ = conn.Write([]byte("x"))
				_ = conn.Close()
				_ = conn.Close() // a second close frees no second slot
			}()
		}
	}()

	const clients = 30

	for range clients {
		served.Go(func() {
			conn, err := net.Dial("tcp", inner.Addr().String())

			if err != nil {
				t.Error(err)

				return
			}

			defer func() { _ = conn.Close() }()

			_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))

			if _, err := conn.Read(make([]byte, 1)); err != nil {
				t.Error("a connection over the limit was not served:", err)
			}
		})
	}

	served.Wait()

	if got := most.Load(); got > 3 || got < 2 {
		t.Fatalf("%d connections at once, the limit is 3", got)
	}

	if full.Load() == 0 {
		t.Fatal("full was never called")
	}
}

// Close ends an Accept waiting for a free slot.
func TestLimitListenerClose(t *testing.T) {
	inner, err := net.Listen("tcp", "127.0.0.1:0")

	if err != nil {
		t.Fatal(err)
	}

	listener := LimitListener(inner, 1, nil)
	client, err := net.Dial("tcp", inner.Addr().String())

	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = client.Close() }()

	if _, err := listener.Accept(); err != nil {
		t.Fatal(err)
	}

	accepted := make(chan error, 1)

	go func() {
		_, err := listener.Accept()

		accepted <- err
	}()

	time.Sleep(20 * time.Millisecond)

	_ = listener.Close()

	select {
	case err := <-accepted:
		if !errors.Is(err, net.ErrClosed) {
			t.Fatalf("Accept after Close: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Accept still waits after Close")
	}
}
