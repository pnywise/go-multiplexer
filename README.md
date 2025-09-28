# Go Multiplexer Library

The Go Multiplexer Library is a versatile and extensible library for handling multiple protocols (gRPC, HTTP, WebSocket, and raw TCP) on a single port. This library is designed to simplify the development of networked applications by providing a unified interface for managing multiple communication protocols.

## Features

- **gRPC Support**: Easily register and handle gRPC services.
- **HTTP and WebSocket Support**: Serve HTTP endpoints and WebSocket connections.
- **Raw TCP Support**: Handle raw TCP connections with custom handlers.
- **Multiplexing**: Use a single port to handle multiple protocols seamlessly.

## Installation

To use the Go Multiplexer Library, you need to have Go installed. Then, you can install the library using:

```bash
go get github.com/pnywise/go-multiplexer
```

## Usage

Below is an example of how to use the library to handle gRPC, HTTP, WebSocket, and raw TCP protocols on a single port.

### Example

```go
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

type greeterServer struct {
	pb.UnimplementedGreeterServer
}

func (s *greeterServer) SayHello(ctx context.Context, in *pb.HelloRequest) (*pb.HelloReply, error) {
	log.Printf("gRPC request received: Name=%s", in.GetName())
	return &pb.HelloReply{Message: "Hello, " + in.GetName() + "!"}, nil
}

func createHTTPMux() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /hello", func(w http.ResponseWriter, r *http.Request) {
		log.Println("HTTP GET /hello request received")
		fmt.Fprint(w, "Hello from the HTTP server!")
	})

	var upgrader = websocket.Upgrader{}
	mux.HandleFunc("GET /ws", func(w http.ResponseWriter, r *http.Request) {
		log.Println("WebSocket upgrade request received")
		c, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Printf("WebSocket upgrade error: %v", err)
			return
		}
		defer c.Close()
		for {
			mt, message, err := c.ReadMessage()
			if err != nil {
				break
			}
			err = c.WriteMessage(mt, message)
			if err != nil {
				break
			}
		}
	})
	return mux
}

func echoTCPHandler(conn net.Conn) {
	defer conn.Close()
	io.Copy(conn, conn)
}

func main() {
	addr := "127.0.0.1:8080"
	server := multiplexer.New(addr)

	grpcLogger := log.New(os.Stdout, "gRPC: ", log.LstdFlags)
	httpLogger := log.New(os.Stdout, "HTTP: ", log.LstdFlags)
	tcpLogger := log.New(os.Stdout, "TCP:  ", log.LstdFlags)

	grpcServer := grpc.NewServer()
	pb.RegisterGreeterServer(grpcServer, &greeterServer{})
	grpcProtocol := &protocols.GRPCProtocol{Server: grpcServer, Logger: grpcLogger}

	httpServer := &http.Server{Handler: createHTTPMux()}
	httpProtocol := &protocols.HTTPProtocol{Server: httpServer, Logger: httpLogger}

	tcpProtocol := &protocols.TCPProtocol{Handler: echoTCPHandler, Logger: tcpLogger}

	server.Register(grpcProtocol)
	server.Register(httpProtocol)
	server.Register(tcpProtocol)

	if err := server.Run(); err != nil {
		log.Fatalf("Server failed to run: %v", err)
	}
}
```

### Running the Example

1. Save the example code to a file, e.g., `main.go`.
2. Run the application:

   ```bash
   go run main.go
   ```

3. The server will start on `127.0.0.1:8080` and handle gRPC, HTTP, WebSocket, and raw TCP connections.

## Contributing

We welcome contributions to the Go Multiplexer Library! To contribute:

1. Fork the repository.
2. Create a new branch for your feature or bug fix.
3. Write tests for your changes.
4. Ensure all tests pass by running:

   ```bash
   make test
   ```

5. Submit a pull request with a clear description of your changes.

## License

This project is licensed under the MIT License. See the [LICENSE](LICENSE) file for details.