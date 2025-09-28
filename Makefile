# ====================================================================================
# Makefile for the Go Multiplexer Server
#
# This Makefile provides commands to build, run, and manage the project.
# ====================================================================================

# --- Variables ---

# The name of the final binary.
BINARY_NAME=multiplexer_server

# --- Main Targets ---

.PHONY: all run build proto clean help

# The default target, executed when you just run `make`.
all: build
	@echo "Build complete. Binary created: $(BINARY_NAME)"

# Run the server in development mode with live reloading.
# It automatically generates protobuf code before running.
run: proto
	@echo "Starting server in development mode..."
	@go run ./examples/main.go

# Build the production-ready binary.
build: proto
	@echo "Building production binary..."
	@go build -o $(BINARY_NAME) ./examples/main.go

# Run the compiled production binary.
# You must run `make build` before using this target.
start:
	@echo "Starting production server from binary..."
	@./$(BINARY_NAME)


test:
	@echo "Running all tests..."
	@go test ./... -v
	
# --- Utility Targets ---

# Generate Go code from .proto files.
proto:
	@echo "Generating protobuf Go code..."
	@protoc --go_out=. --go_opt=paths=source_relative \
	       --go-grpc_out=. --go-grpc_opt=paths=source_relative \
	       protos/greeter.proto

# Clean up build artifacts and generated files.
clean:
	@echo "Cleaning up generated files..."
	@rm -f $(BINARY_NAME)
	@rm -f protos/*.pb.go
	@echo "Cleanup complete."

# Display help information about the available targets.
help:
	@echo "Available commands:"
	@echo "  make run        - Run the server in development mode."
	@echo "  make start      - Run the compiled production binary."
	@echo "  make proto      - Generate Go code from protobuf definitions."
	@echo "  make clean      - Remove the binary and generated Go files."
	@echo "  make help       - Show this help message."