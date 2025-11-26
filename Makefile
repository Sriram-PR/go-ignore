.PHONY: all test test-race test-cover bench fuzz lint fmt vet clean help

# Default target
all: fmt vet test

# Run all tests
test:
	go test -v ./...

# Run tests with race detector
test-race:
	go test -race -v ./...

# Run tests with coverage
test-cover:
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

# Run benchmarks
bench:
	go test -bench=. -benchmem -run=^$$ ./...

# Run fuzz tests (30 seconds each)
fuzz:
	go test -fuzz=FuzzAddPatterns$ -fuzztime=30s
	go test -fuzz=FuzzMatch$ -fuzztime=30s
	go test -fuzz=FuzzPatternAndPath$ -fuzztime=30s
	go test -fuzz=FuzzGlob$ -fuzztime=30s
	go test -fuzz=FuzzNormalizePath$ -fuzztime=30s
	go test -fuzz=FuzzNormalizeContent$ -fuzztime=30s

# Run git parity tests
test-git:
	go test -v -run TestGitParity ./...

# Run linter (requires golangci-lint)
lint:
	golangci-lint run

# Format code
fmt:
	go fmt ./...

# Run go vet
vet:
	go vet ./...

# Generate test fixtures
fixtures:
	cd testdata && go run fixtures_gen.go

# Clean build artifacts
clean:
	rm -f coverage.out coverage.html benchmark.txt
	rm -rf bin/ dist/
	go clean -testcache

# Full CI check (what CI runs)
ci: fmt vet test test-race lint

# Help
help:
	@echo "Available targets:"
	@echo "  all        - Format, vet, and test (default)"
	@echo "  test       - Run all tests"
	@echo "  test-race  - Run tests with race detector"
	@echo "  test-cover - Run tests with coverage report"
	@echo "  test-git   - Run git parity tests"
	@echo "  bench      - Run benchmarks"
	@echo "  fuzz       - Run fuzz tests (30s each)"
	@echo "  lint       - Run golangci-lint"
	@echo "  fmt        - Format code"
	@echo "  vet        - Run go vet"
	@echo "  fixtures   - Generate test fixtures"
	@echo "  clean      - Clean build artifacts"
	@echo "  ci         - Full CI check"
	@echo "  help       - Show this help"