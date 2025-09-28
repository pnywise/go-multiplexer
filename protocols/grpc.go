package protocols

import (
	"context"
	"errors"
	"log"
	"net"
	"sync"

	"github.com/soheilhy/cmux"
	"google.golang.org/grpc"
)

// GRPCProtocol implements the multiplexer.Protocol interface for gRPC.
type GRPCProtocol struct {
	Server *grpc.Server
	Logger *log.Logger
}

// Matcher returns the cmux.Matcher for gRPC.
func (p *GRPCProtocol) Matcher() cmux.Matcher {
	return cmux.HTTP2HeaderField("content-type", "application/grpc")
}

// Serve starts the gRPC server and handles graceful shutdown.
func (p *GRPCProtocol) Serve(ctx context.Context, wg *sync.WaitGroup, l net.Listener) {
	defer wg.Done()

	go func() {
		<-ctx.Done()
		p.Logger.Println("gRPC server shutting down...")
		p.Server.GracefulStop()
	}()

	p.Logger.Println("gRPC server starting...")
	if err := p.Server.Serve(l); err!= nil &&!errors.Is(err, grpc.ErrServerStopped) {
		p.Logger.Printf("gRPC server failed: %v", err)
	}
	p.Logger.Println("gRPC server stopped.")
}