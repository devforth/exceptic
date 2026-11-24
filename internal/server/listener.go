package server

import (
	"net"
	"sync"
)

// LimitListener accepts at most most connections at once. Over it, Accept
// waits for one to close, so new clients wait in the kernel's accept queue
// instead of being refused; full (may be nil) is called whenever it waits,
// to close idle keep-alive connections.
func LimitListener(listener net.Listener, most int, full func()) net.Listener {
	return &limitListener{Listener: listener, slots: make(chan struct{}, most), full: full, done: make(chan struct{})}
}

type limitListener struct {
	net.Listener

	slots     chan struct{}
	full      func()
	done      chan struct{}
	closeOnce sync.Once
}

func (listener *limitListener) Accept() (net.Conn, error) {
	select {
	case listener.slots <- struct{}{}:
	default:
		if listener.full != nil {
			listener.full()
		}

		select {
		case listener.slots <- struct{}{}:
		case <-listener.done:
			return nil, net.ErrClosed
		}
	}

	conn, err := listener.Listener.Accept()

	if err != nil {
		<-listener.slots

		return nil, err
	}

	return &limitConn{Conn: conn, release: func() { <-listener.slots }}, nil
}

func (listener *limitListener) Close() error {
	listener.closeOnce.Do(func() { close(listener.done) })

	return listener.Listener.Close()
}

// limitConn frees its slot at the first Close.
type limitConn struct {
	net.Conn

	release   func()
	closeOnce sync.Once
}

func (conn *limitConn) Close() error {
	err := conn.Conn.Close()

	conn.closeOnce.Do(conn.release)

	return err
}
