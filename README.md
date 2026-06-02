# go-ignore

[![Go Version](https://img.shields.io/github/go-mod/go-version/Sriram-PR/go-ignore)](https://golang.org/)
[![Go Reference](https://pkg.go.dev/badge/github.com/Sriram-PR/go-ignore.svg)](https://pkg.go.dev/github.com/Sriram-PR/go-ignore)
[![Go Report Card](https://goreportcard.com/badge/github.com/Sriram-PR/go-ignore)](https://goreportcard.com/report/github.com/Sriram-PR/go-ignore)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

A minimal, zero-dependency Go library for matching file paths against `.gitignore` patterns.

## Features

- **Zero dependencies** — stdlib only
- **Full gitignore syntax** — `*`, `?`, `**`, `!`, `/`, trailing `/`, `\` escapes, `[abc]`, `[a-z]`, negated classes, and all 12 POSIX classes (`[[:alpha:]]` etc.)
- **Filesystem walker** — `WalkDir` / `WalkRepo` traverse a tree, auto-discover nested `.gitignore` files, and prune ignored directories
- **All four gitignore sources** — system, global, `.git/info/exclude`, root `.gitignore` loaded by `LoadRepo`
- **Match provenance** — `MatchResult` reports which file and line produced the matching rule
- **Cross-platform** — Windows backslash normalization, Unix-correct literal backslash, BOM and CRLF/CR/LF auto-handled
- **Thread-safe** — concurrent `Match` and `AddPatterns` supported, including during `WalkDir`
- **Zero-allocation match path** — typical `Match` calls allocate zero heap memory
- **Verified correctness** — git-parity tests against `git check-ignore`, fuzz tests, race-tested
- **Configurable** — case sensitivity, backtrack limits, max-pattern guards for untrusted input

## Why this library?

The Go ecosystem has several `.gitignore` libraries. Here's the case for picking this one — written in 2026 with awareness of the alternatives.

### Correctness, not "mostly works"

Every matcher passes an automated parity test against the system `git check-ignore`. Specifically:

- **`*` does not cross `/` boundaries**, **`**` matches zero-or-more directories**. Several popular libraries get one or both wrong ([sabhiram #21](https://github.com/sabhiram/go-gitignore/issues/21), [monochromegane #12/#13](https://github.com/monochromegane/go-gitignore/issues/12)).
- **`?` and character-class semantics** match git byte-for-byte, including `[!abc]`, `[^abc]`, ranges, and all 12 POSIX classes ([sabhiram #20](https://github.com/sabhiram/go-gitignore/issues/20) is still open here).
- **Parent-excluded negation** — a file cannot be re-included by `!` if a parent directory is already ignored. This subtle spec corner has [an open issue in go-git](https://github.com/go-git/go-git/issues/2112).
- **Trailing-whitespace and escape rules** — `foo\ ` preserves a trailing space; `\!foo` is a literal `!foo`; trailing backslashes are reported as warnings rather than silently matching nothing.
- **Windows-authored content** — UTF-8 BOM and CRLF/CR line endings auto-normalized.

### Operational quality for high-throughput tools

- **Zero allocations on the match path** for paths up to 32 segments (typical). Verified by benchmarks in `benchmark_test.go`; preserved through the v0.x line.
- **Zero dependencies** — drop it into any project without weighing the import graph.
- **Walker that doesn't mutate its receiver** — `WalkDir` can be called repeatedly without rule accumulation; safe for long-lived matchers in services.
- **Cross-platform CI** — Windows, macOS, Linux, on Go 1.25 and 1.26.
- **Resource limits** — `MaxPatterns`, `MaxPatternLength`, `MaxBacktrackIterations` for safe handling of untrusted input.

### How it compares

| | go-ignore | [sabhiram](https://github.com/sabhiram/go-gitignore) | [git-pkgs](https://pkg.go.dev/github.com/git-pkgs/gitignore) | [go-git](https://pkg.go.dev/github.com/go-git/go-git/v5/plumbing/format/gitignore) |
|---|:---:|:---:|:---:|:---:|
| Zero dependencies | ✓ | ✓ | ✓ | ✗ (billy.Filesystem) |
| `**` semantics correct | ✓ | [#21](https://github.com/sabhiram/go-gitignore/issues/21) | ✓ | ✓ |
| `?` and character classes | ✓ (all 12 POSIX) | [#20](https://github.com/sabhiram/go-gitignore/issues/20) | ✓ | ✓ |
| Parent-excluded negation | ✓ | — | ✓ | [#2112](https://github.com/go-git/go-git/issues/2112) |
| Filesystem walker | ✓ | ✗ | ✓ | partial |
| Nested `.gitignore` auto-discovery | ✓ | ✗ (callers hand-roll) | ✓ | ✓ |
| Match provenance (Source + Line) | ✓ | ✗ (bool only) | ✓ | ✗ |
| Zero-alloc match path | ✓ | — | — | — |
| Git-parity test suite | ✓ | — | — | — |
| Cross-platform CI (Win/Mac/Linux) | ✓ | — | — | ✓ |

`✓` = supported, `✗` = unsupported, `—` = not advertised / not verified, issue number = known open bug.

### When *not* to use this library

- You need `.dockerignore`, `.npmignore`, or another similar-but-different file format. This library implements git's spec; the other formats overlap heavily but diverge on edge cases.
- You need full git repository access (reading the index, writing packfiles, etc.). Use [`go-git`](https://github.com/go-git/go-git) — `go-ignore` is path-matching only.
- You want a one-line regex-equivalent. Just use `path/filepath.Match` from the stdlib if your needs are simple.

## Installation

```bash
go get github.com/Sriram-PR/go-ignore
```

Requires Go 1.25 or later.

## Quick Start

```go
package main

import (
    "fmt"
    "os"

    ignore "github.com/Sriram-PR/go-ignore"
)

func main() {
    m := ignore.New()

    // IMPORTANT: Add .git/ explicitly if you want Git-like behavior
    // (the library intentionally doesn't auto-ignore .git/)
    m.AddPatterns("", []byte(".git/\n"))

    // Load .gitignore (BOM and CRLF automatically handled)
    content, _ := os.ReadFile(".gitignore")
    m.AddPatterns("", content)
    for _, w := range m.Warnings() {
        fmt.Printf("Warning line %d: %s\n", w.Line, w.Message)
    }

    // Check paths (thread-safe)
    fmt.Println(m.Match("node_modules/foo.js", false)) // true (ignored)
    fmt.Println(m.Match("src/main.go", false))         // false (not ignored)
    fmt.Println(m.Match("build", true))                // true if "build/" pattern exists
}
```

## Usage

### Basic Matching

```go
m := ignore.New()
m.AddPatterns("", []byte(`
*.log
build/
node_modules/
!important.log
`))

// Check if path should be ignored
// Second parameter indicates if path is a directory
m.Match("debug.log", false)           // true
m.Match("important.log", false)       // false (negated)
m.Match("build", true)                // true (directory)
m.Match("build/output.js", false)     // true (inside ignored dir)
m.Match("src/main.go", false)         // false
```

### Nested .gitignore Files

```go
m := ignore.New()

// Root .gitignore
m.AddPatterns("", []byte("*.log\n"))

// src/.gitignore (patterns scoped to src/)
m.AddPatterns("src", []byte("*.tmp\n!keep.tmp\n"))

// src/lib/.gitignore (patterns scoped to src/lib/)
m.AddPatterns("src/lib", []byte("*.bak\n"))

// Results:
m.Match("test.log", false)           // true (root pattern)
m.Match("src/test.log", false)       // true (root pattern applies everywhere)
m.Match("src/test.tmp", false)       // true (src pattern)
m.Match("src/keep.tmp", false)       // false (negated in src)
m.Match("test.tmp", false)           // false (src pattern doesn't apply at root)
m.Match("src/lib/test.bak", false)   // true (src/lib pattern)
```

### Debug Why a Path Matches

```go
m := ignore.New()
m.AddPatterns("", []byte(`
*.log
!important.log
build/
`))

result := m.MatchWithReason("debug.log", false)
fmt.Printf("Ignored: %v\n", result.Ignored)   // true
fmt.Printf("Rule: %s\n", result.Rule)         // *.log
fmt.Printf("Line: %d\n", result.Line)         // 1
fmt.Printf("Negated: %v\n", result.Negated()) // false

result = m.MatchWithReason("important.log", false)
fmt.Printf("Ignored: %v\n", result.Ignored)   // false (re-included)
fmt.Printf("Rule: %s\n", result.Rule)         // !important.log
fmt.Printf("Negated: %v\n", result.Negated()) // true
```

### Case-Insensitive Matching (Windows/macOS)

```go
// For case-insensitive filesystems
m := ignore.NewWithOptions(ignore.MatcherOptions{
    CaseInsensitive: true,
})

m.AddPatterns("", []byte("BUILD/\n*.LOG\n"))

m.Match("build", true)      // true
m.Match("Build", true)      // true
m.Match("BUILD", true)      // true
m.Match("test.log", false)  // true
m.Match("test.LOG", false)  // true
```

### Parse Warnings

```go
// Option 1: Collect warnings via Warnings()
m := ignore.New()
m.AddPatterns("", content)
for _, w := range m.Warnings() {
    fmt.Printf("Line %d: %s - %s\n", w.Line, w.Pattern, w.Message)
}

// Option 2: Configure a handler at construction (fixed for the matcher's lifetime)
m := ignore.NewWithOptions(ignore.MatcherOptions{
    WarningHandler: func(w ignore.ParseWarning) {
        log.Printf("[%s] line %d: %s", w.BasePath, w.Line, w.Message)
    },
})
m.AddPatterns("", content)       // warnings go to handler
m.AddPatterns("src", srcContent) // w.BasePath == "src" for warnings from this call
```

### Windows Path Support

On Windows, backslashes in paths are automatically normalized to forward slashes.
On Linux/macOS, backslashes are treated as literal filename characters (matching Git's behavior).

```go
m := ignore.New()
m.AddPatterns("", []byte("src/build/\n*.log\n"))

// On Windows: backslashes are converted to forward slashes
m.Match("src\\build\\output.exe", false)  // true on Windows
m.Match("src\\main.go", false)            // false on Windows

// On all platforms: forward slashes always work
m.Match("src/build/output.exe", false)    // true
m.Match("src/main.go", false)             // false
```

### Concurrent Usage

```go
m := ignore.New()
m.AddPatterns("", content)

// Safe to call Match from multiple goroutines
var wg sync.WaitGroup
for _, path := range paths {
    wg.Add(1)
    go func(p string) {
        defer wg.Done()
        if m.Match(p, false) {
            // handle ignored file
        }
    }(path)
}
wg.Wait()
```

### Global Gitignore

Load the user's global gitignore file (`core.excludesFile` or `~/.config/git/ignore`) with a single call:

```go
m := ignore.New()

// Load global patterns (core.excludesFile → $XDG_CONFIG_HOME/git/ignore → ~/.config/git/ignore)
if err := m.AddGlobalPatterns(); err != nil {
    log.Fatal(err)
}

// Then load repo-level .gitignore as usual
content, _ := os.ReadFile(".gitignore")
m.AddPatterns("", content)

m.Match("debug.log", false) // may be ignored by global patterns
```

If the global gitignore file does not exist, `AddGlobalPatterns` returns nil (no error).

### Repository Exclude File

Load the repository's `.git/info/exclude` file:

```go
m := ignore.New()

// Load .git/info/exclude patterns
if err := m.AddExcludePatterns(".git"); err != nil {
    log.Fatal(err)
}

// Then load repo-level .gitignore as usual
content, _ := os.ReadFile(".gitignore")
m.AddPatterns("", content)
```

If the exclude file does not exist, `AddExcludePatterns` returns nil (no error).

### Load a Working Tree in One Call

`LoadRepo` is a convenience constructor that pre-loads the four standard gitignore sources for a working tree in git's precedence order (lowest first):

1. The system gitignore (`git config --system core.excludesFile`)
2. The user's global gitignore (`core.excludesFile` / XDG)
3. `<repoRoot>/.git/info/exclude`
4. `<repoRoot>/.gitignore` (root scope)

```go
m, err := ignore.LoadRepo(".", ignore.MatcherOptions{})
if err != nil {
    log.Fatal(err)
}
// Paths passed to Match must be relative to repoRoot:
m.Match("build/output.js", false)
```

Missing files are silently skipped; only real read failures are returned. Nested per-directory `.gitignore` files are **not** walked by `LoadRepo` — use `WalkDir` / `WalkRepo` (below) if you want nested discovery, or call `AddPatternsFromFile(basePath, path)` for each subdirectory explicitly.

### Walking a Working Tree

`WalkDir` (method on `Matcher`) and `WalkRepo` (standalone) walk a directory tree and call your callback only for files and directories that are **not** ignored. They auto-load nested `.gitignore` files as they descend, and prune `.git/` and any ignored directory without descending.

The standard one-shot use case — "walk this repo, skip ignored files":

```go
err := ignore.WalkRepo(".", ignore.MatcherOptions{}, func(path string, d fs.DirEntry, err error) error {
    if err != nil { return err }
    if d.IsDir() { return nil }
    process(path)
    return nil
})
```

Power user — pre-load extra patterns on top of `LoadRepo`'s sources, then walk:

```go
m, _ := ignore.LoadRepo(".", ignore.MatcherOptions{})
m.AddPatterns("", []byte(".envrc\n")) // extra root-scope patterns
m.WalkDir(".", walkFn)
```

`WalkDir` does **not** mutate the receiver: discovered nested rules are scoped to the call. The receiver can be safely reused for subsequent walks or `Match` queries.

#### Iterator form (`Files` / `RepoFiles`)

For the common "iterate non-ignored files" case, `Files` (method) and `RepoFiles` (standalone) return Go 1.23+ range-over-func iterators that yield only files — directories are not surfaced. Breaking out of the loop stops traversal cleanly.

```go
for path, err := range ignore.RepoFiles(".", ignore.MatcherOptions{}) {
    if err != nil { return err }
    process(path)
}
```

Same nested-discovery and pruning behavior as `WalkDir`. If you need directory entries too, use `WalkDir` directly.

#### Walking an `fs.FS` (`WalkDirFS`)

For in-memory tests, `embed.FS` content, or any custom `fs.FS` implementation, use `WalkDirFS`:

```go
fsys := fstest.MapFS{
    ".gitignore":   {Data: []byte("*.log\n")},
    "keep.txt":     {Data: []byte("x")},
    "debug.log":    {Data: []byte("x")},
    "sub/file.txt": {Data: []byte("x")},
}
m := ignore.New()
m.AddPatterns("", []byte("*.log\n"))
m.WalkDirFS(fsys, ".", func(path string, d fs.DirEntry, err error) error {
    if err != nil { return err }
    fmt.Println(path) // forward-slash, fs.WalkDir convention
    return nil
})
```

Same matching, discovery, and pruning behavior as `WalkDir`; only the filesystem backend differs. Paths supplied to `fn` always use forward slashes (the `fs.WalkDir` convention), regardless of host OS.

`FilesFS` is the iterator form of `WalkDirFS`:

```go
for path, err := range m.FilesFS(fsys, ".") {
    if err != nil { return err }
    process(path)
}
```

### Streaming Patterns from an `io.Reader`

#### Streaming Patterns from an `io.Reader`

`AddPatternsReader` accepts an `io.Reader` so callers don't have to `io.ReadAll` themselves:

```go
f, _ := os.Open(".gitignore")
defer f.Close()
if err := m.AddPatternsReader("", f); err != nil {
    log.Fatal(err)
}
```

Read errors are wrapped and returned; rules are added on a successful read. Equivalent to `io.ReadAll` followed by `AddPatterns`.

## Supported Syntax

| Pattern | Meaning | Example Matches |
|---------|---------|-----------------|
| `foo` | File/dir anywhere | `foo`, `src/foo`, `a/b/foo` |
| `/foo` | File/dir at root only | `foo` (not `src/foo`) |
| `foo/` | Directory only | `foo/` dir and contents |
| `*.log` | Wildcard extension | `debug.log`, `error.log` |
| `foo*bar` | Wildcard middle | `foobar`, `fooxyzbar` |
| `**/logs` | Any depth prefix | `logs`, `src/logs`, `a/b/logs` |
| `logs/**` | Everything inside | `logs/a`, `logs/a/b/c` |
| `a/**/b` | Any depth middle | `a/b`, `a/x/b`, `a/x/y/z/b` |
| `!pattern` | Negate previous | Re-includes matched files |
| `#comment` | Comment line | Ignored |
| `\#file` | Literal # | Matches `#file` |
| `\!file` | Literal ! | Matches `!file` |
| `?.txt` | Single byte | `a.txt`, `b.txt` (not `ab.txt`) |
| `[abc]` | Character class | `a`, `b`, or `c` |
| `[a-z]` | Character range | Any lowercase letter |
| `[!abc]` or `[^abc]` | Negated class | Any char except `a`, `b`, `c` |
| `[[:alpha:]]` | POSIX class | Any letter |
| `\*` | Literal * | Matches `*` (escaped wildcard) |

**Note:** `?` and character classes (`[...]`) operate on raw bytes, not Unicode code points, consistent with Git's behavior. A multi-byte UTF-8 character requires multiple `?` to match.

### Pattern Anchoring

- **No slash** → matches anywhere: `temp` matches `temp`, `src/temp`, `a/b/temp`
- **Contains slash** → anchored to base: `src/temp` matches only `src/temp`
- **Leading slash** → anchored to root: `/temp` matches only `temp` at root
- **Trailing slash** → directories only: `build/` matches `build/` dir and all contents
- **`**/` prefix** → floats (not anchored): `**/temp` matches anywhere

## Limitations

The library does **not** automatically ignore `.git/` — add it explicitly if needed.

## Path Normalization Notes

Paths containing `..` are resolved internally via `path.Clean` so callers cannot bypass scoped patterns (e.g., `src/../secret.txt` is matched as `secret.txt`, not as a path inside `src/`). Paths that resolve above the repository root (e.g., `../escape.txt`) are treated as non-matching.

## Resource Limits

Default limits prevent resource exhaustion from untrusted input:

| Limit | Default | Description |
|-------|---------|-------------|
| `MaxPatterns` | 100,000 | Total rules a Matcher will hold. Excess rules are dropped with a warning. |
| `MaxPatternLength` | 4,096 | Maximum length of a single pattern line. Longer lines are skipped with a warning. |
| `MaxBacktrackIterations` | 10,000 | Iteration budget shared across all rules per `Match` call. Prevents pathological `**` patterns from causing excessive CPU. |

`MaxPatterns` and `MaxPatternLength` accept `-1` to disable the limit entirely (not recommended for untrusted input). `MaxBacktrackIterations` accepts `-1` as well, but it does **not** disable the cap — it raises the soft limit to the exported constant `HardMaxBacktrackIterations` (10,000,000). Truly unlimited backtracking is intentionally not offered: pathological glob patterns can blow up exponentially and hang a process, so the library always enforces a ceiling.

There is also a non-configurable, exported constant `MaxPathDepth` (4096) that caps the segment count of paths passed to `Match` / `MatchWithReason`. Paths exceeding this depth short-circuit to "no match" without evaluating any rules. The cap exists because the spec-required parent-excluded negation walk is inherently O(M·N²) in path depth — without it, pathological inputs (constructible by fuzzers or malicious callers) could peg CPU for minutes. Realistic filesystem paths are nowhere near 4096 segments.

## API Reference

### Types

```go
type Matcher struct { /* ... */ }

type MatcherOptions struct {
    WarningHandler         WarningHandler // Default: nil (warnings collected via Warnings())
    MaxBacktrackIterations int            // Default: 10000; -1 raises soft limit to HardMaxBacktrackIterations (10M); truly unlimited not offered
    CaseInsensitive        bool           // Default: false
    MaxPatterns            int            // Default: 100000, use -1 for unlimited
    MaxPatternLength       int            // Default: 4096, use -1 for unlimited
}

type MatchResult struct {
    Ignored  bool   // Final decision
    Matched  bool   // Whether any rule matched
    Rule     string // The matching pattern
    Source   string // Path to source file (empty if AddPatterns called without source info)
    BasePath string // Directory scope of the matching rule
    Line     int    // Line number (1-indexed)
}

func (r MatchResult) Negated() bool // derived: r.Matched && !r.Ignored

type ParseWarning struct {
    Pattern  string
    Message  string
    Line     int
    BasePath string
}

type WarningHandler func(warning ParseWarning)
```

### Functions

```go
func New() *Matcher
func NewWithOptions(opts MatcherOptions) *Matcher
func LoadRepo(repoRoot string, opts MatcherOptions) (*Matcher, error)
func WalkRepo(root string, opts MatcherOptions, fn fs.WalkDirFunc) error
func RepoFiles(root string, opts MatcherOptions) iter.Seq2[string, error]

func (m *Matcher) AddPatterns(basePath string, content []byte)
func (m *Matcher) AddPatternsWithSource(basePath, source string, content []byte)
func (m *Matcher) AddPatternsReader(basePath string, r io.Reader) error
func (m *Matcher) AddPatternsFromFile(basePath, path string) error
func (m *Matcher) AddSystemPatterns() error
func (m *Matcher) AddGlobalPatterns() error
func (m *Matcher) AddExcludePatterns(gitDir string) error
func (m *Matcher) Match(path string, isDir bool) bool
func (m *Matcher) MatchWithReason(path string, isDir bool) MatchResult
func (m *Matcher) WalkDir(root string, fn fs.WalkDirFunc) error
func (m *Matcher) WalkDirFS(fsys fs.FS, root string, fn fs.WalkDirFunc) error
func (m *Matcher) Files(root string) iter.Seq2[string, error]
func (m *Matcher) FilesFS(fsys fs.FS, root string) iter.Seq2[string, error]
func (m *Matcher) Warnings() []ParseWarning
func (m *Matcher) RuleCount() int
```

## Performance

Benchmarked on Intel i9-14900HX (Go 1.26, linux/amd64; median of 2× `-benchtime=3s` runs):

| Operation | Time | Allocs |
|-----------|------|--------|
| Simple match (hit) | ~83ns | 0 |
| Simple match (miss) | ~105ns | 0 |
| Directory-only pattern | ~71ns | 0 |
| Match with `**` (shallow) | ~85ns | 0 |
| Match with `**` (20-segment path) | ~276ns | 0 |
| Match with `**` (32-segment path) | ~450ns | 0 |
| Match with `**` (50-segment path) | ~1.2µs | 0 |
| `MatchWithReason` | ~74ns | 0 |
| Negation rule | ~85ns | 0 |
| Nested .gitignore (scoped basePath) | ~200ns | 0 |
| Match against 200 rules (hit) | ~2.9µs | 0 |
| Match against 200 rules (miss, all evaluated) | ~4.5µs | 0 |
| Pathological `**` (bounded by budget) | ~210–370ns | 0 |
| Case-insensitive (lowercase path) | ~86ns | 0 |
| Case-insensitive (uppercase path, requires `ToLower`) | ~248ns | 1 (24 B) |
| Concurrent match (RLock contention) | ~80ns | 0 |
| Glob matching (simple/prefix/complex) | ~33–84ns | 0 |
| Character class (`[abc]`, ranges, POSIX) | ~27–48ns | 0 |
| Path normalization | ~46ns | 0 |
| `AddPatterns` (small / medium / large) | ~1.2µs / ~5µs / ~97µs | 14 / 56 / 905 |

The backtrack budget (`MaxBacktrackIterations`, default 10,000) is **shared across all rules** within a single `Match` call. A matcher with many complex `**` patterns will exhaust the budget faster than one with few patterns. When the budget is exceeded, remaining rules are treated as non-matching. Increase the budget via `MatcherOptions` if needed.

## Thread Safety

`Matcher` is safe for concurrent use:

- Multiple goroutines can call `Match` simultaneously (read lock)
- `AddPatterns` can be called concurrently with `Match` (write lock)

**Best practice**: Batch all `AddPatterns` calls before starting concurrent `Match` operations to minimize lock contention.

## Stability Guarantees

Starting with v1.0, this library follows [semantic versioning](https://semver.org/) strictly. The compatibility contract within the v1.x line:

**Will NOT change:**

- Names and signatures of exported types (`Matcher`, `MatcherOptions`, `MatchResult`, `ParseWarning`, `WarningHandler`).
- Names and signatures of exported functions (`New`, `NewWithOptions`, `LoadRepo`, `WalkRepo`, `RepoFiles`).
- Names and signatures of exported methods on `*Matcher` and `MatchResult`.
- Names and types of exported constants (`DefaultMaxPatterns`, `DefaultMaxPatternLength`, `DefaultMaxBacktrackIterations`, `HardMaxBacktrackIterations`, `MaxPathDepth`).
- Default values for the three `Default*` constants.
- Documented matching semantics — `**` globstar, character-class behavior, parent-excluded negation, anchoring rules, basePath scoping.

**May change (non-breaking only):**

- New exported types, functions, methods, fields, and constants may be added.
- Existing defensive limit constants (`HardMaxBacktrackIterations`, `MaxPathDepth`) may be **raised** to accommodate larger workloads, but will not be lowered (callers relying on a current ceiling will keep working).
- Performance improvements that do not alter observable matching behavior.
- Documentation, error messages, warning text.

**Reserves the right to change (rare, with notes in the release):**

- Bug fixes that correct previously-incorrect behavior. If the prior behavior diverged from git's spec (verified via the parity tests), the fix is shipped under a minor or patch tag with a documented note in `RELEASE.md`. Callers depending on the buggy behavior should pin the affected version.

**Not part of the API:**

- Unexported types, methods, functions, fields, and constants — anything not visible to a caller via `import`. The internal `walkBackend`, `addPatternsFromSource`, `rule`, `segment`, `matchContext`, etc. may be refactored or removed at any time.
- The exact text of `ParseWarning.Message` strings (the structured fields — `Line`, `Pattern`, `BasePath` — are stable; the human-readable `Message` may be reworded).

If a v1.x release breaks any of the "will not change" guarantees, it is a bug — please file an issue.

## Contributing

Contributions are welcome! Please ensure:

1. All tests pass: `go test ./...`
2. Race detector passes: `go test -race ./...`
3. New features include tests
4. Code is formatted: `go fmt ./...`

## License

MIT — see [LICENSE](LICENSE) for details.
