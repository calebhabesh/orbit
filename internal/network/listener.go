package network

import (
	"net"
	"sync"
)

const MaxDirectIncomingConnections = MaxActivePeerSlots * MaxDirectAttempts

// Direct TCP admission is bounded before net/http allocates TLS goroutines.
// Existing finite HTTP/TLS deadlines retire idle accepted connections. The
// backlog belongs to the kernel; Orbit never queues unbounded accepted sockets.
type directListener struct {
	net.Listener
	slots chan struct{}
	done  chan struct{}
	once  sync.Once
}

func boundedDirectListener(listener net.Listener) net.Listener {
	return &directListener{Listener: listener, slots: make(chan struct{}, MaxDirectIncomingConnections), done: make(chan struct{})}
}
func (l *directListener) Accept() (net.Conn, error) {
	select {
	case <-l.done:
		return nil, net.ErrClosed
	case l.slots <- struct{}{}:
	}
	conn, err := l.Listener.Accept()
	if err != nil {
		<-l.slots
		return nil, err
	}
	return &incomingDirectConn{Conn: conn, release: func() { <-l.slots }}, nil
}
func (l *directListener) Close() error {
	l.once.Do(func() { close(l.done) })
	return l.Listener.Close()
}

type incomingDirectConn struct {
	net.Conn
	release func()
	once    sync.Once
}

func (c *incomingDirectConn) Close() error { err := c.Conn.Close(); c.once.Do(c.release); return err }
