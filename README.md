# go-ignore

[![Go Version](https://img.shields.io/github/go-mod/go-version/Sriram-PR/go-ignore)](https://golang.org/)
[![Go Reference](https://pkg.go.dev/badge/github.com/Sriram-PR/go-ignore.svg)](https://pkg.go.dev/github.com/Sriram-PR/go-ignore)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

Zero-dependency `.gitignore` matching for Go, verified byte-for-byte against `git check-ignore`.

## Highlights

- **Correct** — full gitignore syntax (`*`, `?`, `**`, `!`, `/`, `\` escapes, character classes, and all 12 POSIX classes), with an automated parity test suite against the system `git`.
- **Batteries included** — a filesystem walker with nested `.gitignore` auto-discovery and directory pruning, plus one-call loading of all four gitignore sources (system, global, `.git/info/exclude`, and the repo root).
- **Fast and safe** — a zero-allocation match path, safe for concurrent use, fuzz- and race-tested.
- **Zero dependencies** — the standard library only.

## Install

```bash
go get github.com/Sriram-PR/go-ignore
```

Requires Go 1.26 or later.

## Quick Start

```go
package main

import (
	"fmt"

	ignore "github.com/Sriram-PR/go-ignore"
)

func main() {
	m := ignore.New()
	m.AddPatterns("", []byte("*.log\nbuild/\n!important.log\n"))

	fmt.Println(m.Match("debug.log", false))     // true
	fmt.Println(m.Match("important.log", false)) // false (negated)
	fmt.Println(m.Match("build/app.js", false))  // true  (inside an ignored dir)
	fmt.Println(m.Match("src/main.go", false))   // false
}
```

The second argument to `Match` reports whether the path is a directory. The library does **not** auto-ignore `.git/` — add `.git/` yourself if you want git-like walking behavior.

### Walk a repository

`WalkRepo` loads the standard gitignore sources for a working tree, then walks it, calling your function only for entries that are **not** ignored and pruning ignored (and `.git/`) directories as it descends:

```go
import "io/fs"

err := ignore.WalkRepo(".", ignore.MatcherOptions{}, func(path string, d fs.DirEntry, err error) error {
	if err != nil {
		return err
	}
	if !d.IsDir() {
		process(path)
	}
	return nil
})
```

The iterator form is `for path, err := range ignore.RepoFiles(".", ignore.MatcherOptions{})`, which yields only non-ignored files.

### Explain a match

`MatchWithReason` reports which rule decided a path, and where it came from:

```go
r := m.MatchWithReason("debug.log", false)
fmt.Println(r.Ignored, r.Rule, r.Line) // true *.log 1
```

## Supported Syntax

| Pattern | Meaning | Example matches |
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
| `\#file` | Literal `#` | Matches `#file` |
| `\!file` | Literal `!` | Matches `!file` |
| `?.txt` | Single byte | `a.txt`, `b.txt` (not `ab.txt`) |
| `[abc]` | Character class | `a`, `b`, or `c` |
| `[a-z]` | Character range | Any lowercase letter |
| `[!abc]` / `[^abc]` | Negated class | Any char except `a`, `b`, `c` |
| `[[:alpha:]]` | POSIX class | Any letter |
| `\*` | Literal `*` | Matches `*` |

**Anchoring:** a pattern with no `/` matches at any depth; a leading `/` anchors to the root; a trailing `/` matches directories only; a `**/` prefix floats to any depth.

**Note:** `?` and character classes operate on raw bytes, not Unicode code points (matching git). A multi-byte UTF-8 character needs multiple `?` to match.

## How It Compares

The Go ecosystem has several `.gitignore` libraries. This one prioritizes spec-correctness (proven by parity tests against git) and drop-in operational quality:

| | go-ignore | others |
|---|:---:|:---:|
| `*` / `**` boundary semantics correct | ✓ | varies |
| `?` and character classes (incl. all 12 POSIX) | ✓ | varies |
| Parent-excluded negation | ✓ | varies |
| Filesystem walker + nested discovery | ✓ | varies |
| Match provenance (source + line) | ✓ | varies |
| Zero-allocation match path | ✓ | — |
| Git-parity test suite | ✓ | — |

If you need `.dockerignore`/`.npmignore` (similar but not identical formats), full git repository access, or only trivial glob matching, a different tool may fit better — this library implements git's ignore spec and nothing more.

## Documentation

Full API reference, runnable examples, resource-limit tuning for untrusted input, thread-safety guarantees, and path-normalization rules are on **[pkg.go.dev](https://pkg.go.dev/github.com/Sriram-PR/go-ignore)**.

Key entry points:

- `New` / `NewWithOptions` — construct a matcher (options cover case sensitivity and resource limits).
- `LoadRepo` — load system + global + `.git/info/exclude` + root `.gitignore` in one call.
- `WalkDir` / `WalkRepo` / `Files` / `RepoFiles` / `WalkDirFS` / `FilesFS` — walk a tree (OS or `fs.FS`), skipping ignored files.
- `AddPatterns` / `AddPatternsReader` / `AddPatternsFromFile` (plus `…WithSource` variants) — add rules from bytes, a reader, or a file.
- `Match` / `MatchWithReason` — test a path, optionally with match provenance.

## Stability Guarantees

Semantic versioning. Within the v1.x line:

- Exported identifiers keep their names and signatures. New ones may be added.
- The `Default*` limits keep their values. `HardMaxBacktrackIterations` and `MaxPathDepth` may be raised, never lowered.
- Documented matching semantics stay the same, except bug fixes where the library disagreed with `git check-ignore`. Those ship in a patch or minor release with a note in [CHANGELOG.md](CHANGELOG.md).
- Unexported code and the wording of `ParseWarning.Message` are not part of the API.

## Contributing

Contributions are welcome. Please run `go test ./...`, `go test -race ./...`, and `go fmt ./...`, and include tests for new behavior.

## License

MIT — see [LICENSE](LICENSE).
