.PHONY: build test bench clean install lint fmt

# Binary name
BINARY=pdfcmp

# Build flags
LDFLAGS=-ldflags "-s -w"

# Default target
all: build

# Build the binary
build:
	go build $(LDFLAGS) -o $(BINARY) ./cmd/pdfcmp

# Build with race detector
build-race:
	go build -race -o $(BINARY) ./cmd/pdfcmp

# Install to GOPATH/bin
install:
	go install $(LDFLAGS) ./cmd/pdfcmp

# Run tests
test:
	go test -v ./...

# Run tests with race detector
test-race:
	go test -race -v ./...

# Run benchmarks
bench:
	go test -bench=. -benchmem ./bench/...

# Run specific benchmark
bench-hash:
	go test -bench=Hash -benchmem ./bench/...

bench-byte:
	go test -bench=Byte -benchmem ./bench/...

bench-compare:
	go test -bench=Compare -benchmem ./bench/...

# Run all benchmarks with count for statistical significance
bench-full:
	go test -bench=. -benchmem -count=5 ./bench/... | tee benchmark_results.txt

# Profile CPU
profile-cpu:
	go test -bench=BenchmarkPHash_Large -cpuprofile=cpu.prof ./bench/...
	go tool pprof -http=:8080 cpu.prof

# Profile memory
profile-mem:
	go test -bench=BenchmarkPHash_Large -memprofile=mem.prof ./bench/...
	go tool pprof -http=:8080 mem.prof

# Format code
fmt:
	go fmt ./...

# Lint code
lint:
	golangci-lint run

# Clean build artifacts
clean:
	rm -f $(BINARY)
	rm -f cpu.prof mem.prof
	rm -f benchmark_results.txt
	rm -rf bench/fixtures/*

# Tidy dependencies
tidy:
	go mod tidy

# Download dependencies
deps:
	go mod download

# Check for outdated dependencies
outdated:
	go list -u -m all

# Show module graph
deps-graph:
	go mod graph

# Generate test coverage
coverage:
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

# Quick development cycle
dev: fmt test build

# Release build (optimized)
release:
	CGO_ENABLED=1 go build $(LDFLAGS) -o $(BINARY) ./cmd/pdfcmp

# Cross-compile (requires appropriate toolchains)
build-linux:
	GOOS=linux GOARCH=amd64 CGO_ENABLED=1 go build $(LDFLAGS) -o $(BINARY)-linux-amd64 ./cmd/pdfcmp

# Help
help:
	@echo "Available targets:"
	@echo "  build       - Build the binary"
	@echo "  install     - Install to GOPATH/bin"
	@echo "  test        - Run tests"
	@echo "  bench       - Run benchmarks"
	@echo "  bench-full  - Run benchmarks with count=5"
	@echo "  profile-cpu - Profile CPU usage"
	@echo "  profile-mem - Profile memory usage"
	@echo "  fmt         - Format code"
	@echo "  lint        - Lint code"
	@echo "  clean       - Clean build artifacts"
	@echo "  coverage    - Generate coverage report"
	@echo "  dev         - Format, test, and build"
