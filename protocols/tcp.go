package protocols

import (
	"context"
	"errors"
	"log"
	"net"
	"sync"

	"github.com/soheilhy/cmux"
)

// TCPHandler defines the function signature for handling raw TCP connections.
type TCPHandler func(conn net.Conn)

// TCPProtocol implements the multiplexer.Protocol interface for raw TCP.
type TCPProtocol struct {
	Handler TCPHandler
	Logger  *log.Logger
}

// Matcher returns the cmux.Matcher for any connection not matched by other protocols.
func (p *TCPProtocol) Matcher() cmux.Matcher {
	return cmux.Any()
}

// Serve starts the raw TCP server and handles graceful shutdown.
func (p *TCPProtocol) Serve(ctx context.Context, wg *sync.WaitGroup, l net.Listener) {
	defer wg.Done()

	go func() {
		<-ctx.Done()
		p.Logger.Println("TCP server shutting down...")
		l.Close()
	}()

	p.Logger.Println("Raw TCP server starting...")
	for {
		conn, err := l.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				p.Logger.Println("TCP server stopped.")
				return
			default:
				if !errors.Is(err, net.ErrClosed) {
					p.Logger.Printf("TCP accept error: %v", err)
				}
				return
			}
		}
		go p.Handler(conn)
	}
}
