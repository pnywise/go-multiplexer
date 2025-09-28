// Package protocols_test provides comprehensive testing for the TCP protocol
// implementation, including lifecycle management, error handling, and connection
// handling capabilities.
package protocols

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/soheilhy/cmux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockTCPListener implements net.Listener with configurable error injection
// capabilities for testing various failure scenarios in the TCP server.
//
// Features:
// - Configurable Accept() error injection
// - Thread-safe Close() operation
// - Proper error propagation
type mockTCPListener struct {
	net.Listener               // Embedded real listener for basic functionality
	closeOnce    sync.Once     // Ensures Close() is called only once
	closeCh      chan struct{} // Signals listener closure
	acceptErr    error         // Configurable error returned by Accept()
}

// newMockTCPListener creates a new mock TCP listener wrapping an existing
// listener with configurable error behavior.
//
// Parameters:
//   - l: The underlying net.Listener to wrap
//   - acceptErr: Optional error to return on Accept()
//
// Returns:
//   - *mockTCPListener: A configured mock listener
func newMockTCPListener(l net.Listener, acceptErr error) *mockTCPListener {
	return &mockTCPListener{
		Listener:  l,
		closeCh:   make(chan struct{}),
		acceptErr: acceptErr,
	}
}

// Accept implements net.Listener.Accept() with added error injection and
// closure detection capabilities.
//
// Returns:
//   - net.Conn: An accepted connection, or nil on error
//   - error: Either the injected error, net.ErrClosed, or an underlying Accept error
func (m *mockTCPListener) Accept() (net.Conn, error) {
	select {
	case <-m.closeCh:
		// Simulate standard closed listener behavior
		return nil, net.ErrClosed
	default:
		if m.acceptErr != nil {
			return nil, m.acceptErr
		}
		return m.Listener.Accept()
	}
}

// Close implements a thread-safe listener closure that ensures
// proper cleanup and signals all goroutines waiting on closeCh.
//
// Returns:
//   - error: Any error from closing the underlying listener
func (m *mockTCPListener) Close() error {
	m.closeOnce.Do(func() {
		close(m.closeCh)
	})
	return m.Listener.Close()
}

// echoHandler implements a simple TCP connection handler that echoes
// back any received data to the client. It ensures proper connection
// cleanup by closing the connection when the handler returns.
//
// Parameters:
//   - conn: The TCP connection to handle
//
// This handler is useful for basic connection testing and demonstrating
// proper connection lifecycle management.
func echoHandler(conn net.Conn) {
	defer conn.Close()
	io.Copy(conn, conn)
}

// TestTCPProtocol_Matcher verifies that the TCP protocol's Matcher method
// returns a valid cmux.Matcher that can be used for connection matching.
//
// Test cases:
// - Confirms that Matcher() returns a non-nil matcher
// - Implicitly verifies that cmux.Any() is being used
//
// This test runs in parallel with other tests to improve test execution time.
func TestTCPProtocol_Matcher(t *testing.T) {
	t.Parallel()
	p := &TCPProtocol{}
	assert.NotNil(t, p.Matcher(), "Matcher() should return a non-nil value")
}

// TestTCPProtocol_Serve_Lifecycle performs a comprehensive test of the TCP
// protocol's server lifecycle, including startup, connection handling,
// and graceful shutdown.
//
// Test scenarios:
// 1. Server startup and initialization
// 2. Client connection establishment
// 3. Data transmission and echo verification
// 4. Graceful shutdown process
//
// This test verifies:
// - Proper server initialization
// - Connection acceptance
// - Data handling through echo functionality
// - Clean shutdown with context cancellation
//
// The test runs in parallel with other tests for efficient execution.
func TestTCPProtocol_Serve_Lifecycle(t *testing.T) {
	t.Parallel()

	// Set up logging capture
	var logBuf bytes.Buffer
	logger := log.New(&logBuf, "", 0)

	// Initialize protocol with echo handler
	protocol := &TCPProtocol{
		Handler: echoHandler,
		Logger:  logger,
	}

	// Create TCP listener on an available port
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err, "Failed to listen on a free port")

	// Set up context for controlled shutdown
	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	wg.Add(1)

	// Start the server
	go protocol.Serve(ctx, &wg, l)

	// Allow server initialization
	time.Sleep(50 * time.Millisecond)

	// Test 1: Verify connection and echo functionality
	clientConn, err := net.Dial("tcp", l.Addr().String())
	require.NoError(t, err, "Failed to connect to the TCP server")

	// Send test message
	testMessage := "hello raw tcp"
	_, err = clientConn.Write([]byte(testMessage))
	require.NoError(t, err)

	// Prepare to read response
	readBuf := make([]byte, len(testMessage))
	_, err = io.ReadFull(clientConn, readBuf)
	require.NoError(t, err, "Failed to read echo response")
	assert.Equal(t, testMessage, string(readBuf), "Echo response did not match sent message")
	clientConn.Close()

	// 2. Trigger graceful shutdown
	cancel()
	wg.Wait() // Wait for Serve to return. l.Close() is called inside Serve.

	// 3. Verify log output
	logOutput := logBuf.String()
	assert.Contains(t, logOutput, "Raw TCP server starting...")
	assert.Contains(t, logOutput, "TCP server shutting down...")
	assert.Contains(t, logOutput, "TCP server stopped.")

	// 4. Ensure server is stopped
	_, err = net.Dial("tcp", l.Addr().String())
	assert.Error(t, err, "Should not be able to connect after server is stopped")
}

func TestTCPProtocol_Serve_Failure(t *testing.T) {
	t.Parallel()
	var logBuf bytes.Buffer
	logger := log.New(&logBuf, "", 0)
	expectedErr := errors.New("synthetic accept error")

	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err, "Failed to create listener")
	l.Close()
	mockL := newMockTCPListener(l, expectedErr)

	protocol := &TCPProtocol{
		Handler: echoHandler,
		Logger:  logger,
	}

	var wg sync.WaitGroup
	wg.Add(1)
	// Serve should start, fail on Accept, log the specific error, and exit.
	protocol.Serve(context.Background(), &wg, mockL)
	wg.Wait()

	logOutput := logBuf.String()
	expectedLog := fmt.Sprintf("TCP accept error: %v", expectedErr)
	assert.Contains(t, logOutput, expectedLog, "Log should contain the specific accept error")
}

func TestTCPProtocol_IntegrationWithCMUX(t *testing.T) {
	t.Parallel()
	var tcpLogBuf bytes.Buffer
	tcpLogger := log.New(&tcpLogBuf, "tcp: ", 0)

	mainListener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	mux := cmux.New(mainListener)

	// Setup an HTTP protocol to ensure TCP is truly acting as the fallback.
	// Note: setupTestHTTPServer is defined in http_test.go. Go test tools
	// will compile all *_test.go files in a package together.
	httpProtocol := &HTTPProtocol{
		Server: setupTestHTTPServer(),
		Logger: log.New(io.Discard, "", 0), // Suppress HTTP logs for this test
	}
	httpListener := mux.Match(httpProtocol.Matcher())

	// Setup the TCP protocol as the fallback.
	tcpProtocol := &TCPProtocol{
		Handler: echoHandler,
		Logger:  tcpLogger,
	}
	tcpListener := mux.Match(tcpProtocol.Matcher()) // Uses cmux.Any()

	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup

	wg.Add(2) // One for each protocol
	go httpProtocol.Serve(ctx, &wg, httpListener)
	go tcpProtocol.Serve(ctx, &wg, tcpListener)

	go func() {
		err := mux.Serve()
		if err != nil && !errors.Is(err, net.ErrClosed) {
			assert.NoError(t, err, "mux.Serve() failed unexpectedly")
		}
	}()

	time.Sleep(100 * time.Millisecond)

	// 1. Test TCP connection through cmux (should be handled by tcpProtocol)
	clientConn, err := net.Dial("tcp", mainListener.Addr().String())
	require.NoError(t, err, "Failed to make a raw TCP connection via cmux")

	testMessage := "hello multiplexed tcp"
	_, err = clientConn.Write([]byte(testMessage))
	require.NoError(t, err)

	readBuf := make([]byte, len(testMessage))
	_, err = io.ReadFull(clientConn, readBuf)
	require.NoError(t, err)
	assert.Equal(t, testMessage, string(readBuf))
	clientConn.Close()

	// 2. Test HTTP connection to ensure it's routed correctly and not to the TCP handler
	resp, err := http.Get("http://" + mainListener.Addr().String() + "/hello")
	require.NoError(t, err, "HTTP GET request via cmux failed")
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// 3. Trigger shutdown and wait for servers to stop
	cancel()
	mainListener.Close()
	wg.Wait()

	// 4. Verify logs after shutdown
	tcpLogOutput := tcpLogBuf.String()
	assert.Contains(t, tcpLogOutput, "Raw TCP server starting...")
	assert.Contains(t, tcpLogOutput, "TCP server shutting down...")
	assert.Contains(t, tcpLogOutput, "TCP server stopped.")
}
