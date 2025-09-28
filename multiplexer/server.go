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

// Logger is an interface that defines the logging methods used by the server.
// This interface can be implemented by any logging library.
type Logger interface {
	Printf(format string, v ...any)
	Println(v ...any)
	Fatalf(format string, v ...any)
}

// Server is a single-port, multi-protocol server. It is configured by
// registering one or more Protocol implementations.
type Server struct {
	addr      string
	protocols []Protocol
	logger    Logger
}

// New creates a new multiplexer server that listens on the given address.
func New(addr string) *Server {
	return &Server{
		addr:      addr,
		protocols: make([]Protocol, 0),
		logger:    log.New(os.Stdout, "[multiplexer] ", log.LstdFlags),
	}
}

// WithLogger sets a custom logger for the server.
// The logger must implement the Logger interface.
func (s *Server) WithLogger(logger Logger) *Server {
	s.logger = logger
	return s
}

// Register adds a new protocol to the server. Protocols will be matched
// in the order they are registered.
func (s *Server) Register(p Protocol) {
	s.protocols = append(s.protocols, p)
}

// Run starts the multiplexer server and blocks until a shutdown signal is received.
// It handles the graceful shutdown of all registered protocol servers.
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
