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

Two kinds of test compare go-ignore with git:

- `TestGitParity_*` in `git_parity_test.go` are hand-written cases. For a
  specific bug where go-ignore and git disagree, add a case there:

  ```bash
  go test -v -run TestGitParity
  ```

- `TestParity` is the generated harness below. Run it for any change to
  matching or walking.

### Parity harness

`make parity` writes curated and seeded scenarios to temporary git repos and compares `Match`, case-insensitive `Match`, and `WalkDir` with `git check-ignore` and `git ls-files`. Divergences git and go-ignore already disagree on are listed in `testdata/parity/baseline.txt`.

- **Regressions (new divergences):** your change made go-ignore disagree with git somewhere new. Fix it.
- **Resolved:** your change fixed a known divergence. Run `make parity-update` and commit the smaller baseline in the same commit as the fix.

CI runs the harness on every push. To look for new divergence classes, dispatch the workflow with `parity-explore`, which runs a fresh seed and only reports.

### Fuzz Testing

`make fuzz` runs every `Fuzz*` function in the package for 30s each and
`make fuzz-long` for 30m each; `make fuzz FUZZTIME=1m` sets another time.
Every fuzzer runs even if one fails. To fuzz only the surface your change
touches:

```bash
go test -list '^Fuzz' .                              # list the fuzzers
go test -run '^$' -fuzz='^FuzzMatch$' -fuzztime=1m .  # run one
```

A failing input is written to `testdata/fuzz/<FuzzName>/`. Fix the bug and
commit that file with the fix as a regression case. CI uploads the directory
as an artifact when a fuzz job fails.

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
   make ci   # fmt, vet, test, test-race, lint
   ```

   `make ci` needs golangci-lint v2.15.0 or newer (the version CI runs).

4. **Commit with a one-line Conventional Commits message** (`fix:`, `feat:`,
   `docs:`, `test:`, `refactor:`, `perf:`, `ci:`)
   ```bash
   git commit -m "fix: short description of the change"
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
- If it's a Git parity issue, include `git check-ignore -v` output

The bug report form asks for all of this. Report security problems privately,
as described in [SECURITY.md](SECURITY.md).

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

Open an issue with the "question" label.

Thank you for contributing!