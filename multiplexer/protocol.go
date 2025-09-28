package multiplexer

import (
	"context"
	"net"
	"sync"

	"github.com/soheilhy/cmux"
)

// Protocol defines the interface for a network protocol that can be served
// by the multiplexer. Each protocol is responsible for its own matching,
// serving, and graceful shutdown logic.
type Protocol interface {
	// Matcher returns the cmux.Matcher used to identify connections for this protocol.
	Matcher() cmux.Matcher

	// Serve starts the protocol-specific server on the given listener and handles
	// graceful shutdown when the context is canceled. It must call wg.Done()
	// before returning.
	Serve(ctx context.Context, wg *sync.WaitGroup, l net.Listener)
}
