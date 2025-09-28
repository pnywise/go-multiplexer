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

// mockListener is a net.Listener that can be configured to return an error on Accept.
// This is useful for testing server failure scenarios.
type mockGrpcListener struct {
	net.Listener
	closeOnce sync.Once
	closeCh   chan struct{}
	acceptErr error // The error to return on Accept()
}

func newMockGrpcListener(l net.Listener, acceptErr error) *mockGrpcListener {
	return &mockGrpcListener{
		Listener:  l,
		closeCh:   make(chan struct{}),
		acceptErr: acceptErr,
	}
}

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

func (m *mockGrpcListener) Close() error {
	m.closeOnce.Do(func() {
		close(m.closeCh)
	})
	return m.Listener.Close()
}

// setupTestServer creates a new gRPC server and registers a test service.
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
