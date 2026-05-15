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

### v0.5.1

- **Test-only: Windows CI fix** — `TestEdgeCases_Whitespace/escaped_backslash_*` asserted a Unix-only scenario (filename containing a literal `\`) that is unrepresentable on Windows, where backslash is the path separator and gets converted to `/` during normalization. The two cases were split into a new `TestEdgeCases_EscapedBackslash` that skips on Windows. No library code changed.
- **Test-only: FuzzGlob deadline fix** — `FuzzGlob` was using `hardMaxBacktrackIterations` (10M) per input so neither side would short-circuit prematurely, but pathological backtracking patterns could then consume enough wall time to exceed Go's fuzz worker context deadline. Both sides now use `DefaultMaxBacktrackIterations` (10k); pathological inputs exhaust at the same point and are skipped by the existing exhaustion guard.

### v0.5.0

- **Spec compliance: parent-dir-excluded negation** — a path cannot be re-included by `!` if a parent directory is excluded by a prior rule. Verified against `git check-ignore`. Behavior change: callers whose tests asserted the previous (incorrect) re-include will need updates; two such tests in this repo were corrected.
- **Correctness: backtrack budget no longer charges rule enumeration** — previously every rule consumed a tick on entry, so matchers with more rules than `MaxBacktrackIterations` (default 10,000) silently false-negative-ed late rules. The shared budget is now only consumed inside actual backtracking loops; rule iteration and forward progress are free.
- **Robustness: `git config` subprocess bounded by 5s timeout** — a hung or unresponsive `git` binary can no longer stall `AddGlobalPatterns` indefinitely; timeout falls back to XDG path resolution.
- **Defensive: `matchSingleSegment` doubleStar fallback fails closed** — the unreachable branch now returns `false` instead of `true`, so any future refactor that accidentally routes a `**` segment here fails safely rather than reporting a spurious match.
- **Test quality: POSIX class git parity** — six POSIX character class scenarios (`alpha`, `digit`, `alnum`, `upper`/`lower`, `xdigit`, combined) now compared against `git check-ignore` directly.
- **Test quality: CRLF and UTF-8 BOM parity** — Windows-authored `.gitignore` content with `\r\n` line endings or a leading UTF-8 BOM is exercised end-to-end against git.
- **Test quality: escape patterns end-to-end** — added `Match`-level tests for `foo\ ` (literal trailing space) and `foo\\` (literal backslash); previously the trimming logic was unit-tested in isolation.
- **Test quality: per-path subtest isolation** — `compareWithGit` now wraps each path in `t.Run` so one mismatch in a multi-path case produces a clean failure rather than a single error containing all paths.
- **Test quality: `FuzzGlob` invariant** — fuzz target now asserts the fast-path wrapper `matchGlob` agrees with `matchGlobRecursive` on every input, instead of only checking for panics.
- **Test quality: expanded fuzz seed corpora** — `FuzzPatternAndPath` and `FuzzGlob` seeded with audit-discovered edge cases (parent-excluded negation, anchoring, char-class edges, escapes, deep `**`, backtrack-heavy patterns). `.gitignore` updated so any future minimized crash inputs in `testdata/fuzz/` are tracked as regression seeds.
- **Test infra: `gitCheckIgnoreVerbose` correctness** — verbose helper now recognizes leading-`!` rules as negations (`git check-ignore -v` returns exit 0 even for re-includes); spurious `MISMATCH` log lines no longer mask real future regressions.
- **Tooling: dead `lll` exclusion removed** from `.golangci.yml` (linter not in the enable list).
- **Tooling: `Makefile` fuzz target** — stray `$` regex anchor (silently consumed by Make) removed for consistency with `ci.yml`.

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
