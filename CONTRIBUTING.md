# Contributing to go-ignore

Thank you for your interest in contributing! This document provides guidelines and information for contributors.

## Development Setup

1. **Clone the repository**
   ```bash
   git clone https://github.com/Sriram-PR/go-ignore.git
   cd go-ignore
   ```

2. **Ensure Go 1.26+ is installed**
   ```bash
   go version
   ```

3. **Run tests to verify setup**
   ```bash
   go test ./...
   ```

## Making Changes

### Before You Start

- Check existing issues to see if your change is already being discussed
- For significant changes, open an issue first to discuss the approach
- Ensure your change aligns with the project's scope: gitignore matching and walking, stdlib only

### Code Style

- Run `go fmt` before committing
- Follow standard Go conventions
- Keep functions focused and reasonably sized
- Add comments for exported types and functions
- Use meaningful variable names

### Testing

All changes must include appropriate tests:

```bash
# Run all tests
go test ./...

# Run tests with race detector
go test -race ./...

# Run tests with coverage
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out

# Run specific test
go test -v -run TestMatchRule_Basic

# Run benchmarks
go test -bench=. -benchmem
```

### Git Parity

If your change affects matching behavior, verify against Git:

```bash
go test -v -run TestGitParity
```

If you find a discrepancy with Git, add a test case to `git_parity_test.go`.

### Parity harness

`make parity` writes curated and seeded scenarios to temporary git repos and compares `Match`, case-insensitive `Match`, and `WalkDir` with `git check-ignore` and `git ls-files`. Divergences git and go-ignore already disagree on are listed in `testdata/parity/baseline.txt`.

- **Regressions (new divergences):** your change made go-ignore disagree with git somewhere new. Fix it.
- **Resolved:** your change fixed a known divergence. Run `make parity-update` and commit the smaller baseline in the same commit as the fix.

CI runs the harness on every push. To look for new divergence classes, dispatch the workflow with `parity-explore`, which runs a fresh seed and only reports.

### Fuzz Testing

Run fuzz tests to catch edge cases. Each can only run one at a time per
invocation; pick whichever surface your change touches, or run all of them
via `make fuzz` (30s each):

```bash
go test -fuzz=FuzzAddPatterns      -fuzztime=1m .  # pattern parsing
go test -fuzz=FuzzMatch            -fuzztime=1m .  # matching against fixed patterns
go test -fuzz=FuzzPatternAndPath   -fuzztime=1m .  # pattern + path combinations
go test -fuzz=FuzzGlob             -fuzztime=1m .  # glob fast-path vs slow-path invariant
go test -fuzz=FuzzNormalizePath    -fuzztime=1m .  # path normalization
go test -fuzz=FuzzNormalizeContent -fuzztime=1m .  # content normalization (BOM/CRLF)
go test -fuzz=FuzzSegmentMatching  -fuzztime=1m .  # internal segment matcher
go test -fuzz=FuzzConcurrentAccess -fuzztime=1m .  # concurrent Match/AddPatterns
```

## Pull Request Process

1. **Create a feature branch**
   ```bash
   git checkout -b feature/your-feature-name
   ```

2. **Make your changes**
   - Write clean, tested code
   - Update documentation if needed
   - Add/update tests

3. **Run the full test suite**
   ```bash
   go test ./...
   go test -race ./...
   go vet ./...
   golangci-lint run

   # Or, equivalent shortcut:
   make ci
   ```

4. **Commit with a clear message**
   ```bash
   git commit -m "Add feature X: brief description"
   ```

5. **Push and create PR**
   ```bash
   git push origin feature/your-feature-name
   ```

6. **PR Description should include**
   - What the change does
   - Why it's needed
   - How it was tested
   - Any breaking changes

## Reporting Bugs

When reporting bugs, please include:

- Go version (`go version`)
- Operating system
- Minimal reproduction case
- Expected vs actual behavior
- If it's a Git parity issue, include `git check-ignore` output

## Feature Requests

Before requesting features, consider:

- Is it within scope (gitignore matching and walking)?
- Does it maintain zero-dependency principle?
- Would it benefit most users?

## Code of Conduct

- Be respectful and constructive
- Focus on the code, not the person
- Welcome newcomers
- Assume good intentions

## Questions?

Open an issue with the "question" label or start a discussion.

Thank you for contributing!