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
	"google.golang.org/grpc"
)

// GRPCProtocol implements the multiplexer.Protocol interface for gRPC (Google
// Remote Procedure Call) services. It provides support for high-performance,
// bidirectional streaming RPC over HTTP/2.
//
// Features:
// - HTTP/2-based gRPC protocol support
// - Precise protocol detection through content-type headers
// - Built-in graceful shutdown handling
// - Structured logging support
// - Integration with standard gRPC server
type GRPCProtocol struct {
	// Server is the underlying gRPC server instance that processes
	// RPC requests. It should be configured with service registrations
	// before being used.
	Server *grpc.Server

	// Logger provides structured logging capabilities for server
	// lifecycle events and error conditions.
	Logger multiplexer.Logger
}

// Matcher implements the multiplexer.Protocol interface by returning a matcher
// specifically configured for gRPC traffic detection. It uses the HTTP/2
// content-type header to identify gRPC connections.
//
// Returns:
//   - cmux.Matcher: A matcher that identifies gRPC connections by checking
//     for the 'application/grpc' content-type header
//
// This matcher provides high-accuracy gRPC connection detection by looking
// for the standard gRPC protocol indicators in HTTP/2 headers.
func (p *GRPCProtocol) Matcher() cmux.Matcher {
	return cmux.HTTP2HeaderField("content-type", "application/grpc")
}

// Serve implements the multiplexer.Protocol interface by starting the gRPC
// server and managing its lifecycle until shutdown is requested.
// Serve implements the multiplexer.Protocol interface by managing the complete
// lifecycle of the gRPC server, including startup, request handling, and graceful
// shutdown.
//
// Parameters:
//   - ctx: Context for cancellation and shutdown coordination
//   - wg: WaitGroup for shutdown synchronization
//   - l: Listener for accepting gRPC connections
//
// The server lifecycle includes:
// 1. Server initialization and startup
// 2. Processing of gRPC requests
// 3. Graceful shutdown on context cancellation
// 4. Resource cleanup
//
// This implementation ensures:
// - Clean shutdown through GracefulStop
// - Proper error handling and logging
// - Coordinated shutdown with other protocols
func (p *GRPCProtocol) Serve(ctx context.Context, wg *sync.WaitGroup, l net.Listener) {
	// Ensure WaitGroup is decremented when we're done
	defer wg.Done()

	// Set up graceful shutdown handler
	go func() {
		<-ctx.Done()
		p.Logger.Println("gRPC server shutting down...")
		// GracefulStop waits for all active RPCs to finish
		p.Server.GracefulStop()
	}()

	// Start the gRPC server
	p.Logger.Println("gRPC server starting...")
	if err := p.Server.Serve(l); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
		// Log unexpected errors, ignoring normal shutdown
		p.Logger.Printf("gRPC server failed: %v", err)
	}
	p.Logger.Println("gRPC server stopped.")
}
