.PHONY: all build test test-race test-integration test-all lint fmt clean coverage help

# Default target
all: lint build test

# Build the project
build:
	go build -race ./queue/... ./tests/...

# Run unit tests (fast, excludes integration tests)
test:
	go test -race ./queue/... ./tests/...

# Run integration/stress tests (requires -tags=integration)
test-integration:
	go test -race -tags=integration -v ./tests/benchmark_test.go ./tests/extreme_stress_test.go

# Run all tests (unit + integration)
test-all:
	go test -race -tags=integration ./queue/... ./tests/...

# Run linter
lint:
	golangci-lint run

# Format code
fmt:
	go fmt ./...

# Clean build artifacts
clean:
	go clean
	rm -f coverage.out coverage.html

# Generate coverage report
coverage:
	go test -race -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out | grep total

# Show help
help:
	@echo "Available targets:"
	@echo "  build            - Build with race detection"
	@echo "  test             - Run unit tests (fast)"
	@echo "  test-integration - Run stress/integration tests"
	@echo "  test-all         - Run all tests"
	@echo "  lint             - Run golangci-lint"
	@echo "  fmt              - Format code"
	@echo "  clean            - Remove artifacts"
	@echo "  coverage         - Generate coverage report"
