// Package multiplexer provides functionality for serving multiple network protocols
// on a single port using protocol-aware connection multiplexing.
package multiplexer

import (
	"context"
	"net"
	"sync"

	"github.com/soheilhy/cmux"
)

// Protocol defines the interface for a network protocol handler that can be served
// by the multiplexer. Each protocol implementation is responsible for its own
// connection matching logic, request serving, and graceful shutdown handling.
//
// Implementations of this interface should ensure thread-safety and proper
// resource cleanup during shutdown.
type Protocol interface {
	// Matcher returns a cmux.Matcher that identifies network connections
	// belonging to this protocol. The matcher should be deterministic and
	// efficient as it will be called for each incoming connection.
	//
	// Returns:
	//   - cmux.Matcher: A matcher function that returns true for connections
	//     that should be handled by this protocol.
	Matcher() cmux.Matcher

	// Serve starts the protocol-specific server on the provided listener and
	// handles incoming connections until the context is canceled. This method
	// is responsible for proper resource cleanup and graceful shutdown.
	//
	// Parameters:
	//   - ctx: Context for cancellation and timeout control
	//   - wg: WaitGroup for coordinating shutdown across multiple protocols
	//   - l: Network listener for accepting connections
	//
	// The implementation MUST call wg.Done() before returning, regardless of
	// whether the shutdown was graceful or due to an error.
	Serve(ctx context.Context, wg *sync.WaitGroup, l net.Listener)
}
