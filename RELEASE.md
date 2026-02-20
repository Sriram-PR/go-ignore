# Release Checklist

Use this checklist before tagging a new release.

## Pre-Release Verification

### Code Quality

- [ ] All tests pass: `go test ./...`
- [ ] Race detector passes: `go test -race ./...`
- [ ] Go vet clean: `go vet ./...`
- [ ] Linter clean: `golangci-lint run`
- [ ] Code formatted: `go fmt ./...`

### Test Coverage

- [ ] Edge case tests pass: `go test -run TestEdgeCases`
- [ ] Git parity tests pass: `go test -run TestGitParity`
- [ ] Fuzz tests run without crashes:

  ```bash
  go test -fuzz=FuzzAddPatterns -fuzztime=1m
  go test -fuzz=FuzzMatch -fuzztime=1m
  ```

### Benchmarks

- [ ] Benchmarks documented: `go test -bench=. -benchmem`
- [ ] No performance regressions from previous release

### Documentation

- [ ] README.md is complete and accurate
- [ ] All exported functions have GoDoc comments
- [ ] CONTRIBUTING.md is up to date
- [ ] Examples in README work correctly

### Files Present

- [ ] `go.mod` with correct module path
- [ ] `LICENSE` (MIT)
- [ ] `README.md`
- [ ] `CONTRIBUTING.md`
- [ ] `.gitignore`
- [ ] `.golangci.yml`
- [ ] `.github/workflows/ci.yml`

## Release Process

1. **Update version references** (if any)

2. **Final test run**

   ```bash
   make ci
   ```

3. **Create and push tag**

   ```bash
   git tag vX.Y.Z
   git push origin vX.Y.Z
   ```

4. **Create GitHub release**
   - Go to Releases → New Release
   - Select the tag
   - Add release notes with:
     - Features
     - Breaking changes (if any)
     - Bug fixes
     - Contributors

5. **Verify pkg.go.dev**
   - Visit `https://pkg.go.dev/github.com/Sriram-PR/go-ignore@vX.Y.Z`
   - Verify documentation renders correctly

## Version History

### vX.Y.Z

- Character class support: `[abc]`, `[a-z]`, `[!abc]`, `[[:alpha:]]` and all 12 POSIX classes
- Unclosed `[` treated as literal (matches Git behavior)

### v0.2.5

- Global gitignore support via `AddGlobalPatterns()` — resolves `core.excludesFile`, `$XDG_CONFIG_HOME/git/ignore`, or `~/.config/git/ignore`

### v0.2.0

- Spec compliance fixes and backtrack protection improvements
- Documentation and CI updates for Go 1.25+

### v0.1.0 (Initial Release)

- Core gitignore pattern matching
- Support for `*`, `?`, `**`, `!`, `/`, trailing `/`, `\` escapes
- Nested .gitignore file support
- Platform-aware path normalization (Windows backslash conversion, Unix-correct literal backslash)
- Thread-safe concurrent access
- Parse warning diagnostics
- Match debugging with `MatchWithReason`
- Configurable case sensitivity
- Configurable backtrack limits
- Comprehensive test suite
- Fuzz testing
- Git parity tests
- Requires Go 1.25+
