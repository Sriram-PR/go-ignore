# go-ignore

[![Go Reference](https://pkg.go.dev/badge/github.com/Sriram-PR/go-ignore.svg)](https://pkg.go.dev/github.com/Sriram-PR/go-ignore)
[![Go Report Card](https://goreportcard.com/badge/github.com/Sriram-PR/go-ignore)](https://goreportcard.com/report/github.com/Sriram-PR/go-ignore)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

A minimal, zero-dependency Go library for matching file paths against `.gitignore` patterns.

## Features

- **Zero dependencies** — stdlib only
- **Common gitignore syntax** — `*`, `**`, `!`, `/`, trailing `/`
- **Nested .gitignore support** — scoped base paths
- **Cross-platform** — Windows/Unix path normalization
- **Automatic encoding handling** — UTF-8 BOM, CRLF/CR/LF line endings
- **Thread-safe** — concurrent access supported
- **Parse warnings** — malformed pattern diagnostics
- **Match debugging** — `MatchWithReason` for troubleshooting
- **Configurable** — case sensitivity, backtrack limits

## Installation

```bash
go get github.com/Sriram-PR/go-ignore
```

Requires Go 1.24 or later.

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
    if warnings := m.AddPatterns("", content); len(warnings) > 0 {
        for _, w := range warnings {
            fmt.Printf("Warning line %d: %s\n", w.Line, w.Message)
        }
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
fmt.Printf("Negated: %v\n", result.Negated)   // false

result = m.MatchWithReason("important.log", false)
fmt.Printf("Ignored: %v\n", result.Ignored)   // false (re-included)
fmt.Printf("Rule: %s\n", result.Rule)         // !important.log
fmt.Printf("Negated: %v\n", result.Negated)   // true
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
// Option 1: Collect warnings
m := ignore.New()
warnings := m.AddPatterns("", content)
for _, w := range warnings {
    fmt.Printf("Line %d: %s - %s\n", w.Line, w.Pattern, w.Message)
}

// Option 2: Use a handler (must be set BEFORE AddPatterns)
m := ignore.New()
m.SetWarningHandler(func(basePath string, w ignore.ParseWarning) {
    log.Printf("[%s] line %d: %s", basePath, w.Line, w.Message)
})
m.AddPatterns("", content)       // warnings go to handler
m.AddPatterns("src", srcContent) // warnings include "src" as basePath
```

### Windows Path Support

```go
m := ignore.New()
m.AddPatterns("", []byte("src/build/\n*.log\n"))

// Backslashes are automatically normalized
m.Match("src\\build\\output.exe", false)  // true
m.Match("src\\main.go", false)            // false
m.Match(".\\test.log", false)             // true
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

### Pattern Anchoring

- **No slash** → matches anywhere: `temp` matches `temp`, `src/temp`, `a/b/temp`
- **Contains slash** → anchored to base: `src/temp` matches only `src/temp`
- **Leading slash** → anchored to root: `/temp` matches only `temp` at root
- **Trailing slash** → directories only: `build/` matches `build/` dir and all contents
- **`**/` prefix** → floats (not anchored): `**/temp` matches anywhere

## Limitations

The following features are intentionally **not supported**:

- Character classes: `[abc]`, `[0-9]`
- Escape sequences: `\!`, `\#` (except `\#` at line start)
- `.git/info/exclude`
- Global gitignore (`~/.config/git/ignore`)

The library does **not** automatically ignore `.git/` — add it explicitly if needed.

## API Reference

### Types

```go
type Matcher struct { /* ... */ }

type MatcherOptions struct {
    MaxBacktrackIterations int  // Default: 10000, use -1 for unlimited
    CaseInsensitive        bool // Default: false
}

type MatchResult struct {
    Ignored  bool   // Final decision
    Matched  bool   // Whether any rule matched
    Rule     string // The matching pattern
    BasePath string // Source .gitignore location
    Line     int    // Line number (1-indexed)
    Negated  bool   // Was it a negation rule
}

type ParseWarning struct {
    Line    int
    Pattern string
    Message string
}

type WarningHandler func(basePath string, warning ParseWarning)
```

### Functions

```go
func New() *Matcher
func NewWithOptions(opts MatcherOptions) *Matcher

func (m *Matcher) AddPatterns(basePath string, content []byte) []ParseWarning
func (m *Matcher) Match(path string, isDir bool) bool
func (m *Matcher) MatchWithReason(path string, isDir bool) MatchResult
func (m *Matcher) SetWarningHandler(fn WarningHandler)
func (m *Matcher) Warnings() []ParseWarning
```

## Performance

Typical performance on modern hardware:

| Operation | Time |
|-----------|------|
| Simple match | < 500ns |
| Match with ** pattern | < 5µs |
| Match against 100 rules | < 10µs |
| Pathological ** (bounded) | < 1ms |

The library includes backtrack protection (default 10,000 iterations) to prevent pathological patterns from causing excessive CPU usage.

## Thread Safety

`Matcher` is safe for concurrent use:

- Multiple goroutines can call `Match` simultaneously (read lock)
- `AddPatterns` can be called concurrently with `Match` (write lock)

**Best practice**: Batch all `AddPatterns` calls before starting concurrent `Match` operations to minimize lock contention.

## Contributing

Contributions are welcome! Please ensure:

1. All tests pass: `go test ./...`
2. Race detector passes: `go test -race ./...`
3. New features include tests
4. Code is formatted: `go fmt ./...`

## License

MIT — see [LICENSE](LICENSE) for details.
