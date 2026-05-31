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

### v0.9.1

Single bug fix for an adversarial deep-path matching scenario discovered by CI's `FuzzMatch` post-v0.9.0. Users on v0.9.0 should upgrade; the issue is a denial-of-service vector reachable from any caller that runs `Match` / `MatchWithReason` on attacker-controlled paths.

**Bug fixes**

- **Deep-path matching no longer pathological** — `FuzzMatch` triggered "context deadline exceeded" on inputs with many thousands of segments whose leaf matched a negation rule. Root cause: two compounding quadratic terms in `MatchWithReason`. The parent-excluded ancestor walk did `strings.Join` over growing segment prefixes (O(N²) allocations), and for every ancestor it re-evaluated every unanchored rule across all the ancestor's segments (O(M·N²) matching). Fixed in two parts:
  1. The ancestor walk now slices the original `path` string at slash positions instead of joining segments — eliminates the allocation term.
  2. New exported constant `MaxPathDepth` (4096, non-configurable) caps the segment count of paths supplied to `Match` / `MatchWithReason`. Paths exceeding this depth short-circuit to no-match without evaluating any rules. Realistic filesystem paths are nowhere near this limit (Linux `PATH_MAX` is 4096 *bytes*); the cap exists purely to bound DoS-style inputs.

   A 100k-segment path that previously took ~40s now returns immediately. The CI fuzz repro (4 workers × 60s) now executes ~7.8M iterations with no failures.

**Test coverage**

- **`TestEdgeCases_DeepPath_BoundedByDepthCap`** added — asserts both that paths above the cap short-circuit and that under-cap deep paths complete in well under a second.

### v0.9.0

Final polish release before the v1.0 API freeze. All additions are non-breaking; no behavior change for existing code. Focus is closing the API symmetry gap left by v0.8.0 (iterators + fs.FS) and removing the one misleading bit in the MatcherOptions surface.

**Note:** v0.9.0 ships with a known performance issue on adversarial deep-path inputs (DoS-style); upgrade to v0.9.1 if `Match` is invoked on untrusted path input.

**Non-breaking additions**

- **`(*Matcher).Files(root) iter.Seq2[string, error]` and `RepoFiles(root, opts) iter.Seq2[string, error]`** — Go 1.23+ range-over-func iterators that yield non-ignored files (no directories). Built on `WalkDir`/`LoadRepo`, with the same nested `.gitignore` discovery and `.git/` pruning. Breaking out of the loop stops traversal cleanly via `fs.SkipAll`; errors are yielded as `("", err)`. The standard one-liner becomes:
  ```go
  for path, err := range ignore.RepoFiles(".", ignore.MatcherOptions{}) {
      if err != nil { return err }
      process(path)
  }
  ```
- **`(*Matcher).WalkDirFS(fsys fs.FS, root, fn)` and `(*Matcher).FilesFS(fsys fs.FS, root)`** — `fs.FS`-backed counterparts to `WalkDir`/`Files`. Lets the library work with `fstest.MapFS` for tests, `embed.FS` for compiled-in content, and any custom `fs.FS` implementation (WASM, in-memory indexers, etc.). Paths are forward-slash (`fs.WalkDir` convention) regardless of host OS. Implementation shares a single walk engine with `WalkDir` via a small private `walkBackend` indirection — no behavior divergence between the OS and `fs.FS` paths.
- **`(*Matcher).AddPatternsWithSource(basePath, source, content)`** — public form of the internal source-labelled adder. For callers whose patterns originate from a non-file source with a meaningful logical name (embedded config, database row, network response): the supplied label flows through to `MatchResult.Source` for every rule it produces. `AddPatternsFromFile` continues to be the right call for on-disk files.
- **`HardMaxBacktrackIterations` exported constant (10,000,000)** — the absolute ceiling the library enforces on backtracking iterations per `Match` call. Previously an internal `hardMaxBacktrackIterations`; now exported so callers can reason about worst-case CPU and reference it in their own configuration. The `MaxBacktrackIterations` field doc was rewritten to use the constant explicitly: setting it to `-1` raises the soft limit to this ceiling (truly unlimited backtracking is intentionally not offered — pathological glob patterns can blow up exponentially).

**Internal**

- **`walk.go` refactored to a private `walkBackend`** carrying the four filesystem operations that differ between OS and `fs.FS` paths (`walkDir`, `readFile`, `joinPath`, `relPath`). Both `WalkDir` and `WalkDirFS` route through a shared `walkInternal` engine; no public behavior change.

**Documentation**

- **README "Why this library?" positioning section** added — concrete correctness, operational-quality, and feature-completeness bullets, plus a comparison table against `sabhiram/go-gitignore`, `git-pkgs/gitignore`, and `go-git`'s gitignore package (with linked open issues in their trackers rather than vague claims). Closes the "yet another gitignore lib" framing for evaluators landing on the repo.
- **README "Walking a Working Tree" section** picks up subsections for the iterator form (`Files` / `RepoFiles`) and the `fs.FS` variant (`WalkDirFS` / `FilesFS`).
- **README "Resource Limits" paragraph** rewritten to call out the `HardMaxBacktrackIterations` ceiling explicitly rather than describing it as an "internal safety cap".
- **API Reference table** updated with all six v0.9 additions.

### v0.8.0

API-surface additions for the v1.0 push, plus one breaking simplification of `MatchResult`. The additions are the result of a research pass over real downstream consumers (Databricks CLI, gocodewalker, go-git, nektos/act, Pulumi) and recurring asks across the Go gitignore library ecosystem — the headline gap was the absence of a filesystem walker, which every consumer was reimplementing.

**Non-breaking additions**

- **`(*Matcher).WalkDir(root, fn)` and `WalkRepo(root, opts, fn)`** — walk a directory tree, auto-loading nested `.gitignore` files as descent happens and pruning ignored directories (no descent into them). `fn` is called only for entries that survive ignore checks. `WalkDir` does NOT mutate the receiver: discovered rules are scoped to the call via a forked child matcher. `.git/` is always pruned by `WalkDir` regardless of matcher state, to avoid traversing git internals (`Match` itself is unchanged and still requires `.git/` to be added explicitly). `fn` matches `fs.WalkDirFunc`, so callers can switch from `filepath.WalkDir` with a one-line change.
- **`(*Matcher).AddSystemPatterns()`** — fourth standard gitignore source. Resolves via `git config --system core.excludesFile` and loads if the file exists. Missing config or file → returns nil. Symmetric with `AddGlobalPatterns` / `AddExcludePatterns`. `LoadRepo` now invokes this first in its precedence chain (system → global → `.git/info/exclude` → root `.gitignore`).
- **`(*Matcher).AddPatternsFromFile(basePath, path)`** — convenience around `os.ReadFile` + `AddPatterns` that carries the file path as the source label so `MatchResult.Source` identifies it. Closes the provenance gap for callers using neither `LoadRepo` nor a walker.
- **`MatchResult.Source` field** — path to the originating `.gitignore` (or `git config` excludes file) when the matcher knows it. Populated by `LoadRepo`, `WalkDir`'s nested discovery, and the four `Add*Patterns*` loaders that take a path. Empty for rules added via `AddPatterns` / `AddPatternsReader`, which carry only an in-memory blob.

**Breaking changes**

- **`MatchResult.IsIgnored()` and `IsExplicit()` methods removed.** Both were 12-day-old accessors added in v0.7.0 as a layout-stability hedge before the v1.0 freeze. With v1.0 imminent, the hedge is no longer load-bearing and the duplication with the underlying `Ignored` / `Matched` fields is a smell. Migrate by dropping the parens:
  ```go
  // before
  if r.IsIgnored() { ... }
  if r.IsExplicit() { ... }
  // after
  if r.Ignored { ... }
  if r.Matched { ... }
  ```
  `Negated()` is retained because it is a derived value (`Matched && !Ignored`) rather than stored state, and the method form documents the derivation.

**Documentation**

- **README "Walking a Working Tree" section** added, showing the `WalkRepo` one-shot pattern and the `LoadRepo` + `m.WalkDir` power-user shape.
- **API Reference** picks up the new functions and `MatchResult.Source`. `LoadRepo`'s precedence list updated to four sources.

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

**Tooling**

- **`CONTRIBUTING.md` fuzz list synced** — previously listed `FuzzAddPatterns` and `FuzzMatch` only; now lists all eight fuzz targets, matching what CI and `make fuzz` run.
- **`CONTRIBUTING.md` PR checklist now mentions `golangci-lint run` / `make ci`** — the lint step is the actual CI gate but was missing from the contributor instructions.
- **`Makefile` and CI fuzz job pick up `FuzzSegmentMatching` and `FuzzConcurrentAccess`** — both targets existed in `fuzz_test.go` but were never wired into the recurring fuzz runs.

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
