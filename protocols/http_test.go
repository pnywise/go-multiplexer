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

	"github.com/gorilla/websocket"
	"github.com/soheilhy/cmux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockListener is a net.Listener that can be configured to return an error on Accept.
// This is useful for testing server failure scenarios.
type mockHttpListener struct {
	net.Listener
	closeOnce sync.Once
	closeCh   chan struct{}
	acceptErr error // The error to return on Accept()
}

func newMockHttpListener(l net.Listener, acceptErr error) *mockHttpListener {
	return &mockHttpListener{
		Listener:  l,
		closeCh:   make(chan struct{}),
		acceptErr: acceptErr,
	}
}

func (m *mockHttpListener) Accept() (net.Conn, error) {
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

func (m *mockHttpListener) Close() error {
	m.closeOnce.Do(func() {
		close(m.closeCh)
	})
	return m.Listener.Close()
}

// setupTestHTTPServer creates an http.Server with basic handlers for testing.
func setupTestHTTPServer() *http.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /hello", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "world")
	})

	// WebSocket upgrader
	var upgrader = websocket.Upgrader{}
	mux.HandleFunc("GET /ws", func(w http.ResponseWriter, r *http.Request) {
		c, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer c.Close()
		for {
			mt, message, err := c.ReadMessage()
			if err != nil {
				break
			}
			// Echo the message back
			err = c.WriteMessage(mt, message)
			if err != nil {
				break
			}
		}
	})

	return &http.Server{Handler: mux}
}

func TestHTTPProtocol_Matcher(t *testing.T) {
	t.Parallel()
	p := &HTTPProtocol{Server: &http.Server{}}
	assert.NotNil(t, p.Matcher(), "Matcher() should not return nil")
}

func TestHTTPProtocol_Serve_Lifecycle(t *testing.T) {
	t.Parallel()
	var logBuf bytes.Buffer
	logger := log.New(&logBuf, "", 0)

	protocol := &HTTPProtocol{
		Server: setupTestHTTPServer(),
		Logger: logger,
	}

	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err, "Failed to listen on a free port")

	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	wg.Add(1)

	go protocol.Serve(ctx, &wg, l)

	time.Sleep(100 * time.Millisecond) // Allow server to start

	// 1. Check if the server is connectable and responding
	resp, err := http.Get("http://" + l.Addr().String() + "/hello")
	require.NoError(t, err, "HTTP GET request failed")
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, "world", string(body))

	// 2. Trigger graceful shutdown
	cancel()
	wg.Wait()
	l.Close() // Close the listener to release the port

	// 3. Verify log output
	logOutput := logBuf.String()
	assert.Contains(t, logOutput, "HTTP server starting...")
	assert.Contains(t, logOutput, "HTTP server shutting down...")
	assert.Contains(t, logOutput, "HTTP server stopped.")

	// 4. Ensure server is stopped by polling
	var finalErr error
	for i := 0; i < 10; i++ {
		_, finalErr = http.Get("http://" + l.Addr().String() + "/hello")
		if finalErr != nil {
			break // Request failed as expected
		}
		time.Sleep(20 * time.Millisecond)
	}
	assert.Error(t, finalErr, "HTTP server should not be connectable after shutdown")
}

func TestHTTPProtocol_Serve_Failure(t *testing.T) {
	t.Parallel()
	var logBuf bytes.Buffer
	logger := log.New(&logBuf, "", 0)
	expectedErr := errors.New("synthetic network error")

	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err, "Failed to create listener")
	l.Close() // Close the real listener
	mockL := newMockHttpListener(l, expectedErr)

	protocol := &HTTPProtocol{
		Server: setupTestHTTPServer(),
		Logger: logger,
	}

	var wg sync.WaitGroup
	wg.Add(1)
	protocol.Serve(context.Background(), &wg, mockL)
	wg.Wait()

	logOutput := logBuf.String()
	expectedLog := fmt.Sprintf("HTTP server failed: %v", expectedErr)
	assert.Contains(t, logOutput, expectedLog)
}

func TestHTTPProtocol_IntegrationWithCMUX_HTTP(t *testing.T) {
	t.Parallel()
	var logBuf bytes.Buffer
	logger := log.New(&logBuf, "", 0)

	mainListener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	mux := cmux.New(mainListener)

	protocol := &HTTPProtocol{
		Server: setupTestHTTPServer(),
		Logger: logger,
	}
	httpListener := mux.Match(protocol.Matcher())

	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	wg.Add(1)
	go protocol.Serve(ctx, &wg, httpListener)
	go func() {
		err := mux.Serve()
		if err != nil && !errors.Is(err, net.ErrClosed) {
			assert.NoError(t, err, "mux.Serve() failed unexpectedly")
		}
	}()

	time.Sleep(100 * time.Millisecond)

	// Test HTTP request through the main cmux listener
	resp, err := http.Get("http://" + mainListener.Addr().String() + "/hello")
	require.NoError(t, err, "HTTP GET request via cmux failed")
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Shutdown
	cancel()
	mainListener.Close()
	wg.Wait()

	assert.Contains(t, logBuf.String(), "HTTP server stopped.")
}

func TestHTTPProtocol_IntegrationWithCMUX_WebSocket(t *testing.T) {
	t.Parallel()
	mainListener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	mux := cmux.New(mainListener)

	protocol := &HTTPProtocol{
		Server: setupTestHTTPServer(),
		Logger: log.New(io.Discard, "", 0), // Suppress logs for this test
	}
	httpListener := mux.Match(protocol.Matcher())

	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	wg.Add(1)
	go protocol.Serve(ctx, &wg, httpListener)
	go func() {
		_ = mux.Serve()
	}()
	defer func() {
		cancel()
		mainListener.Close()
		wg.Wait()
	}()

	time.Sleep(100 * time.Millisecond)

	// Test WebSocket connection through the main cmux listener
	wsURL := "ws://" + mainListener.Addr().String() + "/ws"
	c, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err, "WebSocket dial via cmux failed")
	defer c.Close()

	// Verify the connection by sending and receiving a message
	testMessage := []byte("hello websocket")
	require.NoError(t, c.WriteMessage(websocket.TextMessage, testMessage))

	_, p, err := c.ReadMessage()
	require.NoError(t, err)
	assert.Equal(t, testMessage, p, "Did not receive expected echo message")
}
