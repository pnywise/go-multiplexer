// Package protocols provides implementations of various network protocols
// that can be used with the multiplexer server. Each protocol implementation
// handles protocol-specific connection matching, serving, and graceful shutdown.
package protocols

import (
	"context"
	"errors"
	"net"
	"sync"

	"github.com/pnywise/go-multiplexer/multiplexer"
	"github.com/soheilhy/cmux"
)

// TCPHandler defines the callback function signature for handling raw TCP connections.
// Implementers should handle the full lifecycle of the connection, including closing.
//
// Parameters:
//   - conn: The established TCP connection to handle
//
// The handler should implement proper error handling and connection cleanup.
// It is executed in its own goroutine, so it can block safely.
type TCPHandler func(conn net.Conn)

// TCPProtocol implements the multiplexer.Protocol interface for raw TCP connections.
// It serves as a catch-all protocol handler, accepting any connections not matched
// by more specific protocol handlers.
//
// This implementation provides:
// - Custom connection handling through a configurable TCPHandler
// - Graceful shutdown coordination
// - Structured logging
type TCPProtocol struct {
	// Handler is called for each new TCP connection
	Handler TCPHandler
	// Logger provides structured logging capabilities
	Logger multiplexer.Logger
}

// Matcher implements the multiplexer.Protocol interface by returning a matcher
// that accepts any remaining connections. This makes TCPProtocol suitable as
// a catch-all handler that should be registered last.
//
// Returns:
//   - cmux.Matcher: A matcher that always returns true
//
// This matcher should typically be registered last, after more specific
// protocol matchers like HTTP or gRPC.
func (p *TCPProtocol) Matcher() cmux.Matcher {
	return cmux.Any()
}

// Serve implements the multiplexer.Protocol interface by accepting and handling
// TCP connections until the context is canceled or an unrecoverable error occurs.
// Serve implements the multiplexer.Protocol interface by managing the lifecycle
// of the TCP server. It accepts incoming connections and dispatches them to the
// configured handler until shutdown is requested.
//
// Parameters:
//   - ctx: Context for cancellation and shutdown coordination
//   - wg: WaitGroup for shutdown synchronization
//   - l: Listener for accepting TCP connections
//
// The server follows this lifecycle:
// 1. Starts accepting connections
// 2. For each accepted connection, launches a handler goroutine
// 3. On context cancellation, stops accepting new connections
// 4. Ensures clean shutdown and resource cleanup
//
// This implementation guarantees:
// - Proper resource cleanup through defer statements
// - Graceful shutdown when context is canceled
// - Clear logging of server lifecycle events
// - Appropriate error handling and reporting
func (p *TCPProtocol) Serve(ctx context.Context, wg *sync.WaitGroup, l net.Listener) {
	// Ensure the WaitGroup is decremented when we're done
	defer wg.Done()

	// Set up graceful shutdown handler
	go func() {
		<-ctx.Done()
		l.Close()
	}()

	p.Logger.Println("Raw TCP server starting...")
	for {
		conn, err := l.Accept()
		if err != nil {
			// The accept loop has been broken, determine the reason
			select {
			case <-ctx.Done():
				// Expected shutdown initiated by context cancellation
				p.Logger.Println("TCP server shutting down...")
			default:
				// Unexpected error, but don't log if it's just ErrClosed
				if !errors.Is(err, net.ErrClosed) {
					p.Logger.Printf("TCP accept error: %v", err)
				}
			}
			// Server is stopped, either gracefully or due to error
			p.Logger.Println("TCP server stopped.")
			return
		}

		// Launch a goroutine to handle the connection
		// The handler is responsible for closing the connection
		go p.Handler(conn)
	}
}
