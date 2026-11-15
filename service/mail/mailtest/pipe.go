package mailtest

import (
	"net"
	"sync"
)

type pipeAddr struct{}

func (pipeAddr) Network() string { return "pipe" }
func (pipeAddr) String() string  { return "pipe" }

type PipeListener struct {
	conns     chan net.Conn
	closed    chan struct{}
	closeOnce sync.Once
}

func NewPipeListener() *PipeListener {
	return &PipeListener{
		conns:  make(chan net.Conn, 16),
		closed: make(chan struct{}),
	}
}

func (l *PipeListener) Accept() (net.Conn, error) {
	select {
	case conn := <-l.conns:
		return conn, nil
	case <-l.closed:
		return nil, net.ErrClosed
	}
}

func (l *PipeListener) Dial() (net.Conn, error) {
	server, client := net.Pipe()

	select {
	case l.conns <- server:
		return client, nil
	case <-l.closed:
		_ = server.Close()
		_ = client.Close()

		return nil, net.ErrClosed
	}
}

func (l *PipeListener) Close() error {
	l.closeOnce.Do(func() {
		close(l.closed)

		for {
			select {
			case conn := <-l.conns:
				_ = conn.Close()
			default:
				return
			}
		}
	})

	return nil
}

func (l *PipeListener) Addr() net.Addr {
	return pipeAddr{}
}
