// Package multiplexer provides a flexible and robust multi-protocol server implementation
// that enables serving multiple network protocols (HTTP, gRPC, TCP, etc.) on a single port.
// It uses protocol-aware connection multiplexing to route incoming connections to the
// appropriate protocol handler.
package multiplexer

import (
	"context"
	"errors"
	"log"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/soheilhy/cmux"
)

// Logger defines the interface for server logging operations. This interface
// is designed to be compatible with most logging libraries while remaining
// minimal and focused.
//
// Implementations of this interface can be provided to customize the server's
// logging behavior, such as log formatting, output destination, or integration
// with existing logging infrastructure.
type Logger interface {
	// Printf formats and logs a message with variable arguments
	Printf(format string, v ...any)

	// Println logs a message with space-separated arguments
	Println(v ...any)

	// Fatalf formats and logs a fatal error message, typically causing program termination
	Fatalf(format string, v ...any)
}

// Server represents a multi-protocol network server that can handle multiple
// protocols on a single port using connection multiplexing. It provides graceful
// shutdown handling and customizable logging.
//
// The server uses protocol matchers to identify and route incoming connections
// to the appropriate protocol handler. Protocols are matched in registration order,
// so more specific matchers should be registered before more general ones.
type Server struct {
	addr      string     // Network address to listen on (host:port)
	protocols []Protocol // Registered protocol handlers
	logger    Logger     // Server logger interface
}

// New creates and initializes a new multiplexer Server instance that will listen
// on the specified network address.
//
// Parameters:
//   - addr: Network address in host:port format (e.g., ":8080" or "localhost:8080")
//
// Returns:
//   - *Server: A new Server instance configured with default settings
//
// The server is initialized with a default logger that writes to stdout. Use
// WithLogger to customize logging behavior.
func New(addr string) *Server {
	return &Server{
		addr:      addr,
		protocols: make([]Protocol, 0),
		logger:    log.New(os.Stdout, "[multiplexer] ", log.LstdFlags),
	}
}

// WithLogger configures the server to use a custom logger implementation.
// This method enables integration with existing logging infrastructure.
//
// Parameters:
//   - logger: Custom logger implementing the Logger interface
//
// Returns:
//   - *Server: The server instance for method chaining
//
// This method follows the builder pattern for fluent configuration.
func (s *Server) WithLogger(logger Logger) *Server {
	s.logger = logger
	return s
}

// Register adds a new protocol handler to the server. Protocols are matched
// in the order they are registered, so more specific matchers should be
// registered before more general ones.
//
// Parameters:
//   - p: Protocol implementation handling connection matching and serving
//
// Protocol handlers must properly implement graceful shutdown and resource
// cleanup when their Serve context is canceled.
func (s *Server) Register(p Protocol) {
	s.protocols = append(s.protocols, p)
}

// Run starts the multiplexer server and blocks until a shutdown signal is received.
// It handles graceful shutdown coordination across all registered protocol handlers.
//
// Returns:
//   - error: nil on successful shutdown, error otherwise
func (s *Server) Run() error {
	l, err := net.Listen("tcp", s.addr)
	if err != nil {
		return err
	}
	defer l.Close()
	s.logger.Printf("Multiplexed server listening on %s", s.addr)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	m := cmux.New(l)
	var wg sync.WaitGroup

	// Iterate over registered protocols to create listeners and start servers.
	for _, p := range s.protocols {
		listener := m.Match(p.Matcher())
		wg.Add(1)
		go p.Serve(ctx, &wg, listener)
	}

	// Start the multiplexer in a separate goroutine.
	go func() {
		if err := m.Serve(); err != nil && !errors.Is(err, net.ErrClosed) {
			s.logger.Fatalf("cmux server failed: %v", err)
		}
	}()

	// Wait for the shutdown signal.
	<-ctx.Done()
	s.logger.Println("Shutdown signal received. Initiating graceful shutdown...")

	// Closing the main listener causes cmux.Serve() to return.
	// The context cancellation triggers the graceful shutdown of each protocol server.
	l.Close()

	// Wait for all server goroutines to complete.
	wg.Wait()
	s.logger.Println("All servers have been shut down. Exiting.")
	return nil
}
