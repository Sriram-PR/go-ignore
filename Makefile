.PHONY: all test test-race test-cover bench fuzz fuzz-long lint fmt vet clean help parity parity-update test-git fixtures conformance conformance-update

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

# Run every Fuzz* function in the package for FUZZTIME each (default 30s).
# All of them run even if one fails; the target fails at the end. A failing input is written to testdata/fuzz/<Name>/.
FUZZTIME ?= 30s

fuzz:
	@list=$$(go test -list '^Fuzz' .) || exit 1; \
	fuzzers=$$(printf '%s\n' "$$list" | grep '^Fuzz') || { echo "no fuzzers found" >&2; exit 1; }; \
	rc=0; for f in $$fuzzers; do \
		go test -run '^$$' -fuzz="^$$f\$$" -fuzztime=$(FUZZTIME) . || rc=1; \
	done; exit $$rc

# Long-form fuzz (30m each), for pre-release
# verification of vX.Y.0 tags.
fuzz-long:
	$(MAKE) fuzz FUZZTIME=30m

# Run git parity tests
test-git:
	go test -v -run TestGitParity ./...

# Run the git parity harness against testdata/parity/baseline.txt
parity:
	GOIGNORE_PARITY=1 go test -run '^TestParity$$' -count=1 .

# Regenerate the parity baseline (commit it with the change that caused it)
parity-update:
	GOIGNORE_PARITY=1 GOIGNORE_PARITY_UPDATE=1 go test -run '^TestParity$$' -count=1 -v .

# Compare go-ignore and other Go gitignore libraries with git (conformance/ module)
conformance:
	cd conformance && go test -run '^TestConformance$$' -count=1 .

# Regenerate conformance/RESULTS.md and the README comparison table
conformance-update:
	cd conformance && CONFORMANCE_UPDATE=1 go test -run '^TestConformance$$' -count=1 .

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
	@echo "  test-git   - Run hand-written git parity cases (TestGitParity_*)"
	@echo "  parity     - Run git parity harness vs baseline"
	@echo "  parity-update - Regenerate the parity baseline"
	@echo "  conformance - Compare with other libraries vs git (conformance/)"
	@echo "  conformance-update - Regenerate the conformance table"
	@echo "  bench      - Run benchmarks"
	@echo "  fuzz       - Run every fuzzer (FUZZTIME each, default 30s)"
	@echo "  fuzz-long  - Run every fuzzer (30m each, pre-release)"
	@echo "  lint       - Run golangci-lint"
	@echo "  fmt        - Format code"
	@echo "  vet        - Run go vet"
	@echo "  fixtures   - Generate test fixtures"
	@echo "  clean      - Clean build artifacts"
	@echo "  ci         - Full CI check"
	@echo "  help       - Show this help"
