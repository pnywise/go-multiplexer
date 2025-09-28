// Package protocols provides implementations of various network protocols
// that can be used with the multiplexer server. Each protocol implementation
// handles protocol-specific connection matching, serving, and graceful shutdown.
package protocols

import (
	"context"
	"errors"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/pnywise/go-multiplexer/multiplexer"
	"github.com/soheilhy/cmux"
)

// HTTPProtocol implements the multiplexer.Protocol interface for serving
// HTTP/1.1 and WebSocket connections. It provides a fully-featured HTTP
// server with customizable routing and middleware support.
//
// Features:
// - Standard HTTP/1.1 protocol support
// - WebSocket connection handling
// - Graceful shutdown with timeout
// - Structured logging
// - Custom server configuration
type HTTPProtocol struct {
	// Server is the underlying http.Server instance that handles HTTP requests.
	// It can be configured with custom handlers, timeouts, and other settings.
	Server *http.Server

	// Logger provides structured logging capabilities for server events
	// and error conditions.
	Logger multiplexer.Logger
}

// Matcher implements the multiplexer.Protocol interface by returning a matcher
// optimized for HTTP/1.1 traffic detection. It uses cmux's HTTP1Fast matcher
// which provides quick and reliable HTTP connection identification.
//
// Returns:
//   - cmux.Matcher: A matcher specifically tuned for HTTP/1.1 connections
//
// The matcher checks for HTTP-specific patterns in the connection's initial
// bytes to identify HTTP traffic efficiently.
func (p *HTTPProtocol) Matcher() cmux.Matcher {
	return cmux.HTTP1Fast()
}

// Serve implements the multiplexer.Protocol interface by starting the HTTP
// server and managing its lifecycle until shutdown is requested.
// Serve implements the multiplexer.Protocol interface by managing the complete
// lifecycle of the HTTP server, including startup, request handling, and graceful
// shutdown.
//
// Parameters:
//   - ctx: Context for cancellation and shutdown coordination
//   - wg: WaitGroup for shutdown synchronization
//   - l: Listener for accepting HTTP connections
//
// The server lifecycle includes:
// 1. Server initialization and startup
// 2. Concurrent request handling
// 3. Graceful shutdown on context cancellation
// 4. Resource cleanup
//
// Shutdown process:
// - Stops accepting new connections
// - Allows in-flight requests to complete (with timeout)
// - Closes all idle connections
// - Ensures proper resource cleanup
func (p *HTTPProtocol) Serve(ctx context.Context, wg *sync.WaitGroup, l net.Listener) {
	// Ensure WaitGroup is decremented when we're done
	defer wg.Done()

	// Set up graceful shutdown handler
	go func() {
		<-ctx.Done()
		p.Logger.Println("HTTP server shutting down...")

		// Create a timeout context for shutdown
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		// Initiate graceful shutdown of the HTTP server
		if err := p.Server.Shutdown(shutdownCtx); err != nil && !errors.Is(err, net.ErrClosed) {
			p.Logger.Printf("HTTP server shutdown error: %v", err)
		} else {
			p.Logger.Println("HTTP server shutdown completed successfully.")
		}

		// Ensure listener is closed
		if err := l.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
			p.Logger.Printf("Listener close error: %v", err)
		} else {
			p.Logger.Println("Listener closed successfully.")
		}
	}()

	// Start the HTTP server
	p.Logger.Println("HTTP server starting...")
	if err := p.Server.Serve(l); err != nil &&
		!errors.Is(err, http.ErrServerClosed) &&
		!errors.Is(err, net.ErrClosed) &&
		!errors.Is(err, cmux.ErrServerClosed) {
		// Log unexpected errors, ignoring expected shutdown errors
		p.Logger.Printf("HTTP server failed: %v", err)
	} else {
		p.Logger.Println("HTTP server exited cleanly.")
	}
	p.Logger.Println("HTTP server stopped.")
}
