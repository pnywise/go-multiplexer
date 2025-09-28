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

// mockTCPListener is a net.Listener that can be configured to return an error on Accept.
// This is useful for testing server failure scenarios.
type mockTCPListener struct {
	net.Listener
	closeOnce sync.Once
	closeCh   chan struct{}
	acceptErr error
}

func newMockTCPListener(l net.Listener, acceptErr error) *mockTCPListener {
	return &mockTCPListener{
		Listener:  l,
		closeCh:   make(chan struct{}),
		acceptErr: acceptErr,
	}
}

func (m *mockTCPListener) Accept() (net.Conn, error) {
	select {
	case <-m.closeCh:
		// Return a standard error for closed listeners
		return nil, net.ErrClosed
	default:
		if m.acceptErr != nil {
			return nil, m.acceptErr
		}
		return m.Listener.Accept()
	}
}

func (m *mockTCPListener) Close() error {
	m.closeOnce.Do(func() {
		close(m.closeCh)
	})
	return m.Listener.Close()
}

// A simple handler that echoes back any data it receives.
func echoHandler(conn net.Conn) {
	defer conn.Close()
	io.Copy(conn, conn)
}

func TestTCPProtocol_Matcher(t *testing.T) {
	t.Parallel()
	p := &TCPProtocol{}
	// This just confirms that cmux.Any() is being called and returns a valid matcher.
	assert.NotNil(t, p.Matcher(), "Matcher() should return a non-nil value")
}

func TestTCPProtocol_Serve_Lifecycle(t *testing.T) {
	t.Parallel()
	var logBuf bytes.Buffer
	logger := log.New(&logBuf, "", 0)

	protocol := &TCPProtocol{
		Handler: echoHandler,
		Logger:  logger,
	}

	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err, "Failed to listen on a free port")

	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	wg.Add(1)

	go protocol.Serve(ctx, &wg, l)

	time.Sleep(50 * time.Millisecond) // Allow server a moment to start the accept loop

	// 1. Test connection and echo functionality
	clientConn, err := net.Dial("tcp", l.Addr().String())
	require.NoError(t, err, "Failed to connect to the TCP server")

	testMessage := "hello raw tcp"
	_, err = clientConn.Write([]byte(testMessage))
	require.NoError(t, err)

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
