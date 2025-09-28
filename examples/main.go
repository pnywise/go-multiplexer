// Package main provides a comprehensive example of using the multiplexer package
// to serve multiple protocols (HTTP, WebSocket, gRPC, and raw TCP) on a single port.
//
// This example demonstrates:
// - Setting up a multiplexer server
// - Configuring multiple protocol handlers
// - Protocol-specific request handling
// - Proper logging and error handling
// - Graceful shutdown handling
//
// Supported Protocols:
// 1. HTTP/1.1 (standard web requests)
// 2. WebSocket (bidirectional communication)
// 3. gRPC (RPC over HTTP/2)
// 4. Raw TCP (fallback protocol)
package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"

	"github.com/pnywise/go-multiplexer/multiplexer"
	"github.com/pnywise/go-multiplexer/protocols"
	pb "github.com/pnywise/go-multiplexer/protos"

	"github.com/gorilla/websocket"
	"google.golang.org/grpc"
)

// --- gRPC Service Implementation ---

// greeterServer implements the gRPC Greeter service, providing a simple
// demonstration of gRPC functionality within the multiplexed server.
type greeterServer struct {
	pb.UnimplementedGreeterServer
}

// SayHello implements the gRPC service method that responds to greeting requests.
//
// Parameters:
//   - ctx: Request context for deadline and cancellation
//   - in: The incoming request containing the caller's name
//
// Returns:
//   - *pb.HelloReply: Response containing the greeting message
//   - error: Any error that occurred during processing
func (s *greeterServer) SayHello(ctx context.Context, in *pb.HelloRequest) (*pb.HelloReply, error) {
	log.Printf("gRPC request received: Name=%s", in.GetName())
	return &pb.HelloReply{Message: "Hello, " + in.GetName() + "!"}, nil
}

// --- HTTP and WebSocket Handler ---

// createHTTPMux sets up an HTTP request multiplexer with handlers for both
// standard HTTP requests and WebSocket connections.
func createHTTPMux() *http.ServeMux {
	mux := http.NewServeMux()

	// Standard HTTP endpoint
	mux.HandleFunc("GET /hello", func(w http.ResponseWriter, r *http.Request) {
		log.Println("HTTP GET /hello request received")
		fmt.Fprint(w, "Hello from the HTTP server!")
	})

	// WebSocket echo endpoint
	var upgrader = websocket.Upgrader{}
	mux.HandleFunc("GET /ws", func(w http.ResponseWriter, r *http.Request) {
		log.Println("WebSocket upgrade request received")
		c, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Printf("WebSocket upgrade error: %v", err)
			return
		}
		defer c.Close()
		log.Println("WebSocket connection established")
		for {
			mt, message, err := c.ReadMessage()
			if err != nil {
				log.Printf("WebSocket read error: %v", err)
				break
			}
			log.Printf("WebSocket received: %s", message)
			err = c.WriteMessage(mt, message)
			if err != nil {
				log.Printf("WebSocket write error: %v", err)
				break
			}
		}
		log.Println("WebSocket connection closed")
	})

	return mux
}

// --- Raw TCP Handler ---

// echoTCPHandler echoes back any data it receives on a raw TCP connection.
func echoTCPHandler(conn net.Conn) {
	remoteAddr := conn.RemoteAddr().String()
	log.Printf("Raw TCP connection received from %s", remoteAddr)
	defer func() {
		conn.Close()
		log.Printf("Raw TCP connection closed for %s", remoteAddr)
	}()

	// Copy data from reader to writer (echo)
	if _, err := io.Copy(conn, conn); err != nil {
		log.Printf("Error in raw TCP echo handler for %s: %v", remoteAddr, err)
	}
}

// --- Main Application ---

func main() {
	// 1. Create a new multiplexer server.
	addr := "127.0.0.1:8080"
	server := multiplexer.New(addr)

	// 2. Create loggers for each protocol for clear output.
	grpcLogger := log.New(os.Stdout, "gRPC: ", log.LstdFlags)
	httpLogger := log.New(os.Stdout, "HTTP: ", log.LstdFlags)
	tcpLogger := log.New(os.Stdout, "TCP:  ", log.LstdFlags)

	// 3. Instantiate and configure the servers for each protocol.
	grpcServer := grpc.NewServer()
	pb.RegisterGreeterServer(grpcServer, &greeterServer{})
	grpcProtocol := &protocols.GRPCProtocol{Server: grpcServer, Logger: grpcLogger}

	httpServer := &http.Server{Handler: createHTTPMux()}
	httpProtocol := &protocols.HTTPProtocol{Server: httpServer, Logger: httpLogger}

	tcpProtocol := &protocols.TCPProtocol{Handler: echoTCPHandler, Logger: tcpLogger}

	// 4. Register the protocols with the multiplexer server.
	// The order of registration matters for matching.
	server.Register(grpcProtocol)
	server.Register(httpProtocol)
	server.Register(tcpProtocol) // cmux.Any() should always be last.

	// 5. Run the server. This is a blocking call that handles the entire lifecycle.
	if err := server.Run(); err != nil {
		log.Fatalf("Server failed to run: %v", err)
	}
}
