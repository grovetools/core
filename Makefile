# Makefile for grove-core (library package)

.PHONY: all build test clean fmt vet lint

# Build all packages (no binary for library)
all: build

build:
	@echo "Building grove-core library..."
	@go build ./...

test:
	@echo "Running tests..."
	@go test -v ./...

clean:
	@echo "Cleaning..."
	@go clean
	@rm -f coverage.out

fmt:
	@echo "Formatting code..."
	@go fmt ./...

vet:
	@echo "Running go vet..."
	@go vet ./...

lint:
	@echo "Running linter..."
	@if command -v golangci-lint > /dev/null; then \
		golangci-lint run; \
	else \
		echo "golangci-lint not installed. Install with: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest"; \
	fi

# Run all checks
check: fmt vet lint test

# Show available targets
help:
	@echo "Available targets:"
	@echo "  make build   - Build the library"
	@echo "  make test    - Run tests"
	@echo "  make clean   - Clean build artifacts"
	@echo "  make fmt     - Format code"
	@echo "  make vet     - Run go vet"
	@echo "  make lint    - Run linter (requires golangci-lint)"
	@echo "  make check   - Run all checks (fmt, vet, lint, test)"
	@echo "  make help    - Show this help"