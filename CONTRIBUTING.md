# Contributing to go-ignore

Thank you for your interest in contributing! This document provides guidelines and information for contributors.

## Development Setup

1. **Clone the repository**
   ```bash
   git clone https://github.com/yourusername/go-ignore.git
   cd go-ignore
   ```

2. **Ensure Go 1.21+ is installed**
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
- Ensure your change aligns with the project's scope (see README limitations)

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

### Fuzz Testing

Run fuzz tests to catch edge cases:

```bash
go test -fuzz=FuzzAddPatterns -fuzztime=1m
go test -fuzz=FuzzMatch -fuzztime=1m
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

- Is it within scope? (See README limitations)
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