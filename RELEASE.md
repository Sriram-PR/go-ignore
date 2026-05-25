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

### v0.7.1

Post-v0.7.0 audit fixes. One observable behavior change in the `WarningHandler` concurrency contract; callers that relied on the library serializing handler dispatch must add their own synchronization (see below).

**Behavior changes**

- **`WarningHandler` may now be invoked concurrently** — v0.7.0 held an internal `handlerMu` while dispatching warnings, which serialized handler calls but also deadlocked any handler that called back into `AddPatterns`. The internal mutex has been removed: handlers must now be safe for concurrent use, but they may freely re-enter the matcher. Callers writing to non-thread-safe state from the handler (e.g., a bare `[]ParseWarning`) need to add their own lock. The new contract is documented on `WarningHandler` and verified by a regression test (`TestMatcher_HandlerReentrancy`).

**Bug fixes**

- **Interior `/./` segments now normalize** — `normalizePath("a/./b")` previously returned `"a/./b"`, so `m.Match("a/./b", false)` did not match the pattern `a/b`. Leading `./` was already stripped; interior `./` segments are now collapsed via `path.Clean`, matching git's own behavior.

**Documentation**

- **README synced with the v0.7.0 API** — code samples and the API Reference section still referenced `SetWarningHandler`, the old `func(basePath, w)` handler signature, and the old `AddPatterns` return value. All four call-sites updated.
- **`LoadRepo` and `AddPatternsReader` documented** — both v0.7.0 additions are now described in `README.md` and `doc.go`; previously they appeared only in this file.
- **`LoadRepo` path contract** — docstring now states explicitly that paths passed to `Match` must be relative to `repoRoot`; the parameter was used only to locate the on-disk files, never stripped from match input.
- **`MaxBacktrackIterations: -1` wording** — README previously implied `-1` disabled the limit; corrected to state that the internal 10,000,000-iteration ceiling still applies. The canonical doc in `ignore.go` already said this.
- **`DefaultMaxPatterns` / `DefaultMaxPatternLength`** — docstring said "silently dropped with a warning"; rephrased to "dropped and a `ParseWarning` is emitted".

**Internal**

- **`MatchWithReason` critical section tightened** — case-insensitive lowering and backtrack-context setup moved outside `m.mu.RLock`. `opts` is fixed at construction and safe to read without the lock; the lock now only covers rule iteration.

**Tests**

- **`MatchResult.IsIgnored` and `IsExplicit` covered** — both accessor methods were at 0% coverage. `TestMatchWithReason_Basic` now asserts them across the no-match / ignored / negated states. Total coverage 96.1% → 96.5%.

### v0.7.0

API-shape cleanup before the v1.0 freeze. **Contains breaking changes** to the public API; migration is mechanical and the migration steps below cover every call-site pattern in the existing test suite.

**Breaking changes**

- **`WarningHandler` moved into `MatcherOptions`** — `SetWarningHandler` is removed. The handler is fixed at construction time, eliminating the "must call before AddPatterns" footgun. To migrate:
  ```go
  // before
  m := ignore.New()
  m.SetWarningHandler(handler)
  // after
  m := ignore.NewWithOptions(ignore.MatcherOptions{WarningHandler: handler})
  ```
- **`WarningHandler` signature change** — `func(basePath string, w ParseWarning)` → `func(w ParseWarning)`. The `basePath` argument was duplicating `w.BasePath`. Read `w.BasePath` inside the handler instead.
- **`AddPatterns` no longer returns `[]ParseWarning`** — the return value was populated only when no handler was configured (asymmetric API). Warnings now flow through exactly one channel: the configured `WarningHandler` if set, otherwise `Warnings()`. To migrate:
  ```go
  // before
  warnings := m.AddPatterns("", content)
  // after
  m.AddPatterns("", content)
  warnings := m.Warnings() // or use a WarningHandler in MatcherOptions
  ```
- **`MatchResult.Negated` field replaced by `(MatchResult).Negated()` method** — the field was a derivable view (`Matched && !Ignored`). The method preserves the accessor ergonomics without storing redundant state. Replace `result.Negated` with `result.Negated()`. New companion methods `IsIgnored()` and `IsExplicit()` provide the same accessor pattern for the `Ignored` and `Matched` fields; callers that want layout-stable accessors should prefer the methods over field reads.

**Non-breaking additions**

- **`LoadRepo(repoRoot, opts) (*Matcher, error)`** — convenience constructor that pre-loads all three standard gitignore sources for a working tree in git's precedence order: global gitignore (lowest), `<repoRoot>/.git/info/exclude`, `<repoRoot>/.gitignore` (highest). Missing files are silently skipped. Nested per-directory `.gitignore` files are not walked; callers that need them should follow up with `AddPatterns` calls scoped to each subdirectory.
- **`AddPatternsReader(basePath, r) error`** — streams pattern content from an `io.Reader` instead of requiring callers to buffer the whole file via `io.ReadAll` first. Read errors are wrapped and returned; rules are added on a successful read.

### v0.6.0

Internal hygiene and documentation polish release. No public API change, no behavior change in matching.

**Internal refactor**
- **Dead field removed** — `segment.hasStar` was set during parse but never read; the same information is carried by `starCount`. Removed the field, its assignment in `parseLines`, and the lone test-struct literal that referenced it.
- **`normalizeBasePath` wrapper inlined** — the wrapper was a transparent forwarder to `normalizePath` (the empty-string early return was redundant with `normalizePath`'s own behavior). The single call site now invokes `normalizePath` directly; the redundant test was removed since `TestNormalizePath` already covers the same inputs.
- **`normalizeContent` CR scan** — replaced `bytes.ContainsRune(content, '\r')` with `bytes.IndexByte(content, '\r') < 0`. `'\r'` is ASCII; no need to decode UTF-8 for the fast-path check.

**Documentation**
- **`newMatchContext` doc** — said "If `maxIter` is `-1`", but the code branch is `maxIter < 0`. Doc now says "any negative value" to match.
- **`maxRecursionDepth` doc** — clarified that the limit applies to every recursive call in `matchSegmentsExact` / `matchSegmentsPrefix`, including the linear tail-recursive segment walk, not just `**` branches.
- **`MatcherOptions.MaxBacktrackIterations` doc** — "-1 for unlimited" was misleading; negative values raise the limit to the internal safety cap (10,000,000 iterations), not true unlimited. Doc now states this explicitly.
- **README `..` handling** — README said the library does not resolve `..`, but `normalizePath` calls `path.Clean` on paths containing `..` and rejects paths that escape the repository root. Corrected the section so callers don't double-clean.
- **`[^abc]` documented** — `matchCharClass` accepts both `!` and `^` as negation inside character classes, but `doc.go` and the README only documented `!`. Both now mention `[^abc]` as an alias.
- **`AddExcludePatterns` example** — added a runnable `ExampleMatcher_AddExcludePatterns` that creates a temp `.git/info/exclude` and exercises the call; pkg.go.dev Examples tab now has an entry for that method.

**Tooling**
- **CI branch triggers** — `.github/workflows/ci.yml` listed `[main, master]` but the repo only has `main`. Dropped `master`.
- **Makefile fuzz targets** — `go test -fuzz=...` lines were missing the package path, so they silently no-op'd from subdirectories. Each line now ends with `.`.

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
