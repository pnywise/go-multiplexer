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

	// Close the listener when the context is canceled
	go func() {
		<-ctx.Done()
		p.Logger.Println("HTTP server shutting down...")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := p.Server.Shutdown(shutdownCtx); err != nil && !errors.Is(err, net.ErrClosed) {
			p.Logger.Printf("HTTP server shutdown error: %v", err)
		} else {
			p.Logger.Println("HTTP server shutdown completed successfully.")
		}
		if err := l.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
			p.Logger.Printf("Listener close error: %v", err)
		} else {
			p.Logger.Println("Listener closed successfully.")
		}
	}()

	p.Logger.Println("HTTP server starting...")
	if err := p.Server.Serve(l); err != nil && !errors.Is(err, http.ErrServerClosed) && !errors.Is(err, net.ErrClosed) && !errors.Is(err, cmux.ErrServerClosed) {
		p.Logger.Printf("HTTP server failed: %v", err)
	} else {
		p.Logger.Println("HTTP server exited cleanly.")
	}
	p.Logger.Println("HTTP server stopped.")
}
