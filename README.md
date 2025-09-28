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

### Using with Standard HTTP

Here's an example using the standard `net/http` package:

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

### Using with Gin Framework

The Gin framework integrates smoothly with the multiplexer as it's compatible with the standard `net/http.Handler` interface. You can use all Gin features including middleware and custom handlers.

Here's how to use the multiplexer with the Gin web framework:

#### Basic Handlers and Middleware

```go
package main

import (
	"context"
	"log"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/pnywise/go-multiplexer/multiplexer"
	"github.com/pnywise/go-multiplexer/protocols"
	pb "github.com/pnywise/go-multiplexer/protos"
	"google.golang.org/grpc"
)

// Custom middleware example
func loggerMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Before request
		startTime := time.Now()

		// Process request
		c.Next()

		// After request
		endTime := time.Now()
		latency := endTime.Sub(startTime)
		log.Printf("[%s] %s %s - %v", c.Request.Method, c.Request.URL.Path, c.ClientIP(), latency)
	}
}

// Custom handler example
type HelloHandler struct {
	message string
}

func (h *HelloHandler) Handle(c *gin.Context) {
	c.JSON(200, gin.H{
		"message": h.message,
		"time":    time.Now(),
	})
}

func setupGinRouter() *gin.Engine {
	r := gin.New() // Create a clean engine without default middleware
	
	// Add custom middleware
	r.Use(gin.Recovery())       // Recovery middleware
	r.Use(loggerMiddleware())   // Our custom logger
	
	// Group routes with common middleware
	api := r.Group("/api")
	{
		api.Use(func(c *gin.Context) {
			// API-specific middleware
			c.Header("API-Version", "1.0")
			c.Next()
		})

		// Handler as a method
		hello := &HelloHandler{message: "Hello from Gin!"}
		api.GET("/hello", hello.Handle)
		
		// Function handler
		api.GET("/echo", func(c *gin.Context) {
			message := c.DefaultQuery("message", "no message provided")
			c.JSON(200, gin.H{"echo": message})
		})
	}

	// WebSocket endpoint with custom middleware
	r.GET("/ws", gin.WrapF(func(w http.ResponseWriter, r *http.Request) {
		var upgrader = websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
		}
		
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Printf("WebSocket upgrade error: %v", err)
			return
		}
		defer conn.Close()
		
		for {
			messageType, p, err := conn.ReadMessage()
			if err != nil {
				return
			}
			if err := conn.WriteMessage(messageType, p); err != nil {
				return
			}
		}
	}))

	return r
}

func main() {
	addr := "127.0.0.1:8080"
	server := multiplexer.New(addr)

	// Setup Gin
	ginHandler := setupGinRouter()
	httpServer := &http.Server{Handler: ginHandler}
	httpProtocol := &protocols.HTTPProtocol{
		Server: httpServer,
		Logger: log.New(os.Stdout, "HTTP: ", log.LstdFlags),
	}

	// Setup gRPC (same as previous example)
	grpcServer := grpc.NewServer()
	pb.RegisterGreeterServer(grpcServer, &greeterServer{})
	grpcProtocol := &protocols.GRPCProtocol{
		Server: grpcServer,
		Logger: log.New(os.Stdout, "gRPC: ", log.LstdFlags),
	}

	server.Register(httpProtocol)
	server.Register(grpcProtocol)

	if err := server.Run(); err != nil {
		log.Fatalf("Server failed to run: %v", err)
	}
}
```

### Using with Fiber Framework

**Important Note**: The Fiber framework uses its own fast HTTP engine and is not directly compatible with the standard `net/http.Handler` interface. To use Fiber with the multiplexer, we need to use an adapter pattern and some specific middleware configurations to ensure compatibility.

Here's how to use the multiplexer with the Fiber web framework:

#### Basic Handlers and Middleware with Compatibility Layer

```go
package main

import (
	"log"
	"net/http"
	"os"

	"github.com/gofiber/fiber/v2"
	"github.com/pnywise/go-multiplexer/multiplexer"
	"github.com/pnywise/go-multiplexer/protocols"
	pb "github.com/pnywise/go-multiplexer/protos"
	"google.golang.org/grpc"
)

// Custom middleware
func loggerMiddleware() fiber.Handler {
	return func(c *fiber.Ctx) error {
		start := time.Now()
		
		err := c.Next()
		
		log.Printf("[%s] %s - %v", c.Method(), c.Path(), time.Since(start))
		return err
	}
}

// Custom handler type
type HelloHandler struct {
	message string
}

func (h *HelloHandler) Handle(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{
		"message": h.message,
		"time":    time.Now(),
	})
}

func setupFiberApp() *fiber.App {
	// Configure Fiber with specific settings for compatibility
	app := fiber.New(fiber.Config{
		DisableStartupMessage: true,
		// Enable strict routing like standard http
		StrictRouting: true,
		// Enable case sensitive routing
		CaseSensitive: true,
		// Custom error handler
		ErrorHandler: func(c *fiber.Ctx, err error) error {
			code := fiber.StatusInternalServerError
			if e, ok := err.(*fiber.Error); ok {
				code = e.Code
			}
			return c.Status(code).JSON(fiber.Map{
				"error": err.Error(),
			})
		},
	})

	// Add global middleware
	app.Use(loggerMiddleware())
	app.Use(func(c *fiber.Ctx) error {
		// Add security headers
		c.Set("X-XSS-Protection", "1; mode=block")
		c.Set("X-Content-Type-Options", "nosniff")
		return c.Next()
	})

	// API routes group
	api := app.Group("/api", func(c *fiber.Ctx) error {
		c.Set("API-Version", "1.0")
		return c.Next()
	})

	// Handler as a method
	hello := &HelloHandler{message: "Hello from Fiber!"}
	api.Get("/hello", hello.Handle)

	// Function handlers
	api.Get("/echo", func(c *fiber.Ctx) error {
		msg := c.Query("message", "no message provided")
		return c.JSON(fiber.Map{"echo": msg})
	})

	// WebSocket handling with Fiber's WebSocket middleware
	app.Use("/ws", func(c *fiber.Ctx) error {
		// Ensure websocket upgrade
		if !websocket.IsWebSocketUpgrade(c) {
			return fiber.ErrUpgradeRequired
		}
		return c.Next()
	})

	app.Get("/ws", websocket.New(func(c *websocket.Conn) {
		for {
			mt, msg, err := c.ReadMessage()
			if err != nil {
				break
			}
			if err := c.WriteMessage(mt, msg); err != nil {
				break
			}
		}
	}))

	return app
}

func main() {
	addr := "127.0.0.1:8080"
	server := multiplexer.New(addr)

	// Setup Fiber
	fiberApp := setupFiberApp()
	httpServer := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fiberApp.Server().Handler.ServeHTTP(w, r)
		}),
	}
	httpProtocol := &protocols.HTTPProtocol{
		Server: httpServer,
		Logger: log.New(os.Stdout, "HTTP: ", log.LstdFlags),
	}

	// Setup gRPC (same as previous example)
	grpcServer := grpc.NewServer()
	pb.RegisterGreeterServer(grpcServer, &greeterServer{})
	grpcProtocol := &protocols.GRPCProtocol{
		Server: grpcServer,
		Logger: log.New(os.Stdout, "gRPC: ", log.LstdFlags),
	}

	server.Register(httpProtocol)
	server.Register(grpcProtocol)

	if err := server.Run(); err != nil {
		log.Fatalf("Server failed to run: %v", err)
	}
}
```

### Running the Examples

1. First, install the required dependencies:
   ```bash
   # For Gin example
   go get github.com/gin-gonic/gin

   # For Fiber example
   go get github.com/gofiber/fiber/v2
   go get github.com/gofiber/websocket/v2  # For WebSocket support
   ```

### Testing the Different Framework Endpoints

After running the server, you can test the endpoints using curl:

```bash
# Test Gin endpoints
curl http://localhost:8080/api/hello
curl "http://localhost:8080/api/echo?message=test"

# Test Fiber endpoints
curl http://localhost:8080/api/hello
curl "http://localhost:8080/api/echo?message=test"

# For WebSocket testing, use a WebSocket client or browser
```

2. Save the example code to a file, e.g., `main.go`.
3. Run the application:

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