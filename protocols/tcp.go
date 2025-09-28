package protocols

import (
	"context"
	"errors"
	"net"
	"sync"

	"github.com/pnywise/go-multiplexer/multiplexer"
	"github.com/soheilhy/cmux"
)

// TCPHandler defines the function signature for handling raw TCP connections.
type TCPHandler func(conn net.Conn)

// TCPProtocol implements the multiplexer.Protocol interface for raw TCP.
type TCPProtocol struct {
	Handler TCPHandler
	Logger  multiplexer.Logger
}

// Matcher returns the cmux.Matcher for any connection not matched by other protocols.
func (p *TCPProtocol) Matcher() cmux.Matcher {
	return cmux.Any()
}

// Serve starts the raw TCP server and handles graceful shutdown deterministically.
func (p *TCPProtocol) Serve(ctx context.Context, wg *sync.WaitGroup, l net.Listener) {
	defer wg.Done()

	go func() {
		<-ctx.Done()
		l.Close()
	}()

	p.Logger.Println("Raw TCP server starting...")
	for {
		conn, err := l.Accept()
		if err != nil {
			// The accept loop has been broken, determine the reason.
			select {
			case <-ctx.Done():
				// This was an expected, graceful shutdown.
				p.Logger.Println("TCP server shutting down...")
			default:
				// This was an unexpected error.
				// We check if it's the common ErrClosed, which we don't need to log.
				if !errors.Is(err, net.ErrClosed) {
					p.Logger.Printf("TCP accept error: %v", err)
				}
			}
			// In either case, the server is now stopped.
			p.Logger.Println("TCP server stopped.")
			return
		}
		go p.Handler(conn)
	}
}
