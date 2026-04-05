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

### v0.4.1

- **Security: basePath bypass via `..`** — `normalizePath` now resolves `..` components via `path.Clean`, preventing `src/../secret.txt` from matching patterns scoped to `src/`
- **Security: exponential backtrack with unlimited budget** — `MaxBacktrackIterations: -1` now caps at 10M iterations instead of running unbounded
- **Security: null byte injection** — paths containing null bytes are now rejected (treated as empty/no match)
- **Bug fix: trailing `**` git parity** — `abc/**` no longer matches `abc` directory itself, only its contents (matches real `git check-ignore` behavior)
- **Bug fix: WarningHandler data race** — concurrent `AddPatterns` calls with a handler no longer race; dispatch serialized via dedicated mutex
- **Performance: zero-alloc deep paths** — `splitPathBuf` increased from `[16]string` to `[32]string`, eliminating heap allocation for paths up to 32 segments
- **Performance: case-insensitive matching** — reduced from 5 allocations to 1 on uppercase input by re-splitting the lowered path instead of lowering each segment
- **Test quality** — added `Match`/`MatchWithReason` consistency invariant to fuzz targets; fixed bogus POSIX class test to verify actual fallback behavior

### v0.4.0

- **Zero-allocation matching** — `Match` and `MatchWithReason` now perform 0 heap allocations for typical paths (down from 2), using stack-buffered path splitting
- **Performance improvements** — removed `defer` for inlining, single-pass wildcard detection, pre-lowered case-insensitive paths, pre-allocated parse buffers
- **Test coverage** — added tests for `MaxPatterns`/`MaxPatternLength` limits, `maxRecursionDepth`, 5 POSIX classes (`blank`/`print`/`graph`/`punct`/`cntrl`), `AddExcludePatterns` permission errors, `matchSegmentsPrefix`, `SetWarningHandler(nil)` reset, fixture match correctness
- **Documentation** — documented resource limits, shared backtrack budget, byte-level `?` matching, `..` path handling, added runnable examples
- **Bug fix** — `gitConfigExcludesFile` now distinguishes expected errors (git not found, key not set) from unexpected errors (permission denied, signals)
- **Refactor** — moved dead `matchGlob` function from production code to test helpers

### v0.3.1

- `.git/info/exclude` support via `AddExcludePatterns()` — completes all three gitignore sources
- Removed outdated known-difference documentation (`\!` escape works correctly)

### v0.3.0

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
