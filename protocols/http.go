package protocols

import (
	"context"
	"errors"
	"log"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/soheilhy/cmux"
)

// HTTPProtocol implements the multiplexer.Protocol interface for HTTP/1.1 and WebSockets.
type HTTPProtocol struct {
	Server *http.Server
	Logger *log.Logger
}

// Matcher returns the cmux.Matcher for HTTP/1.1.
func (p *HTTPProtocol) Matcher() cmux.Matcher {
	return cmux.HTTP1Fast()
}

// Serve starts the HTTP server and handles graceful shutdown.
func (p *HTTPProtocol) Serve(ctx context.Context, wg *sync.WaitGroup, l net.Listener) {
	defer wg.Done()

	go func() {
		<-ctx.Done()
		p.Logger.Println("HTTP server shutting down...")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := p.Server.Shutdown(shutdownCtx); err != nil {
			p.Logger.Printf("HTTP server shutdown error: %v", err)
		}
	}()

	p.Logger.Println("HTTP server starting...")
	if err := p.Server.Serve(l); err != nil && !errors.Is(err, http.ErrServerClosed) {
		p.Logger.Printf("HTTP server failed: %v", err)
	}
	p.Logger.Println("HTTP server stopped.")
}
