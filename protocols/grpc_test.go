// Package protocols_test provides comprehensive testing for the gRPC protocol
// implementation. Tests cover protocol matching, server lifecycle management,
// and error handling scenarios.
package protocols

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/interop/grpc_testing"
)

// mockGrpcListener implements a testable net.Listener with configurable
// error injection and shutdown behavior for gRPC connection testing.
//
// Features:
// - Configurable Accept() error injection
// - Controlled shutdown behavior
// - Thread-safe operations
// - Event signaling through channels
type mockGrpcListener struct {
	net.Listener               // Embedded real listener
	closeOnce    sync.Once     // Ensures single Close() execution
	closeCh      chan struct{} // Signals listener closure
	acceptErr    error         // Configurable Accept() error
}

// newMockGrpcListener creates a new mock gRPC listener with specified
// error behavior for testing server failure scenarios.
//
// Parameters:
//   - l: The underlying net.Listener to wrap
//   - acceptErr: Optional error to return on Accept()
//
// Returns:
//   - *mockGrpcListener: A configured mock listener for testing
func newMockGrpcListener(l net.Listener, acceptErr error) *mockGrpcListener {
	return &mockGrpcListener{
		Listener:  l,
		closeCh:   make(chan struct{}),
		acceptErr: acceptErr,
	}
}

// Accept implements net.Listener.Accept() with added testing capabilities
// such as error injection and closure detection.
//
// Returns:
//   - net.Conn: An accepted connection, or nil on error
//   - error: Either the injected error, closure error, or underlying Accept error
func (m *mockGrpcListener) Accept() (net.Conn, error) {
	select {
	case <-m.closeCh:
		return nil, errors.New("listener closed")
	default:
		if m.acceptErr != nil {
			return nil, m.acceptErr
		}
		return m.Listener.Accept()
	}
}

// Close implements thread-safe listener closure with proper cleanup
// and notification of all goroutines waiting on closeCh.
//
// Returns:
//   - error: Any error from closing the underlying listener
func (m *mockGrpcListener) Close() error {
	m.closeOnce.Do(func() {
		close(m.closeCh)
	})
	return m.Listener.Close()
}

// setupTestServer creates and configures a new gRPC server with a test
// service for integration testing.
func setupTestServer() *grpc.Server {
	server := grpc.NewServer()
	grpc_testing.RegisterTestServiceServer(server, &grpc_testing.UnimplementedTestServiceServer{})
	return server
}

func TestGRPCProtocol_Matcher(t *testing.T) {
	t.Parallel()
	p := &GRPCProtocol{Server: grpc.NewServer()}
	// The main purpose is to ensure a non-nil matcher is returned.
	// The underlying cmux matcher is assumed to be tested by its own library.
	assert.NotNil(t, p.Matcher(), "Matcher() should not return nil")
}

func TestGRPCProtocol_Serve_Failure(t *testing.T) {
	t.Parallel()
	var logBuf bytes.Buffer
	logger := log.New(&logBuf, "test: ", 0)
	expectedErr := errors.New("synthetic accept error")

	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err, "Failed to create listener")
	// We close the real listener immediately and wrap it in a mock that returns an error.
	l.Close()
	mockL := newMockGrpcListener(l, expectedErr)

	protocol := &GRPCProtocol{
		Server: setupTestServer(),
		Logger: logger,
	}

	var wg sync.WaitGroup
	wg.Add(1)
	protocol.Serve(context.Background(), &wg, mockL) // This should return quickly due to the error

	wg.Wait()

	logOutput := logBuf.String()
	expectedLog := fmt.Sprintf("gRPC server failed: %v", expectedErr)
	assert.Contains(t, logOutput, expectedLog)
}
