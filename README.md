# go-ignore

[![Go Version](https://img.shields.io/github/go-mod/go-version/Sriram-PR/go-ignore)](https://golang.org/)
[![Go Reference](https://pkg.go.dev/badge/github.com/Sriram-PR/go-ignore.svg)](https://pkg.go.dev/github.com/Sriram-PR/go-ignore)
[![codecov](https://codecov.io/gh/Sriram-PR/go-ignore/graph/badge.svg)](https://app.codecov.io/gh/Sriram-PR/go-ignore)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

Zero-dependency `.gitignore` matching for Go, verified byte-for-byte against `git check-ignore` (case-sensitive mode; the known case-insensitive differences are under [Differences from git](https://pkg.go.dev/github.com/Sriram-PR/go-ignore#hdr-Differences_from_git)).

## Highlights

- **Correct** — full gitignore syntax (`*`, `?`, `**`, `!`, `/`, `\` escapes, character classes, and all 12 POSIX classes), with an automated parity test suite against the system `git`.
- **Batteries included** — a filesystem walker with nested `.gitignore` auto-discovery, directory pruning, extra ignore files and overrides, single-path checks that apply nested `.gitignore` files, plus one-call loading of the standard gitignore sources (`core.excludesFile` as git resolves it, `.git/info/exclude`, and the repo root).
- **Fast and safe** — no allocations per match for clean paths up to 32 segments, safe for concurrent use, fuzz- and race-tested.
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

`WalkRepo` loads the standard gitignore sources for a working tree, then walks it, calling your function only for entries that are **not** ignored and pruning ignored (and `.git/`) directories as it descends. Like `git ls-files`, it doesn't go inside nested repositories:

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

`RepoEntries` yields directories too, so filters such as hidden directories can prune as they go, and `WalkOptions.Start` walks one subdirectory with the rules above it loaded. `MatcherOptions.IgnoreFiles` reads extra files such as `.aiignore` after each `.gitignore`, and `MatcherOptions.Overrides` holds patterns that beat every ignore file:

```go
opts := ignore.MatcherOptions{
	IgnoreFiles: []string{".aiignore"},
	Overrides:   []string{"*.pem"},
}
for e, err := range ignore.RepoEntries(".", opts, ignore.WalkOptions{Start: "src"}) {
	if err != nil {
		return err
	}
	if e.IsDir() && strings.HasPrefix(e.Name(), ".") {
		e.SkipDir()
		continue
	}
	process(e.Path)
}
```

For a context-packing loop with size and binary filters, see the [EntriesFS context-pack example](https://pkg.go.dev/github.com/Sriram-PR/go-ignore#example-Matcher.EntriesFS-ContextPack).

### Explain a match

`MatchWithReason` reports which rule decided a path, and where it came from:

```go
r := m.MatchWithReason("debug.log", false)
fmt.Println(r.Ignored, r.Rule, r.Line) // true *.log 1
```

### Check one path

A `Matcher` from `LoadRepo` knows only the root `.gitignore`. `OpenTree` also applies every nested `.gitignore` on the way to a path, as `git check-ignore` does, and tells whether the path is a directory by looking at the disk:

```go
tr, err := ignore.OpenTree(".", ignore.MatcherOptions{})
if err != nil {
	return err
}
r, err := tr.Check("src/cache.tmp")
fmt.Println(r.Ignored, r.Source, r.Line) // true src/.gitignore 1
```

Each `.gitignore` is read once per `Tree`, so checking a long list of paths stays cheap.

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

The Go ecosystem has several `.gitignore` libraries. The table counts, by feature, how many scenario paths each one answers the same way as `git check-ignore`. It is generated by `make conformance-update`; [conformance/RESULTS.md](conformance/RESULTS.md) lists the versions and APIs used and every case where a library disagrees with git. Case-insensitive scenarios are left out, since the other libraries don't have that mode; go-ignore's differences there are under [Differences from git](https://pkg.go.dev/github.com/Sriram-PR/go-ignore#hdr-Differences_from_git).

<!-- conformance:start -->
| Feature | go-ignore | git-pkgs | go-git v5 | sabhiram |
|---|--:|--:|--:|--:|
| Whitespace & escapes | 415/415 | 404/415 | 408/415 | 386/415 |
| Line endings & BOM | 22/22 | 20/22 | 20/22 | 20/22 |
| Comments & `!` prefix | 31/31 | 31/31 | 27/31 | 27/31 |
| `*` and `?` | 334/334 | 334/334 | 317/334 | 279/334 |
| `**` | 736/736 | 731/736 | 600/736 | 678/736 |
| Directories & negation | 135/135 | 135/135 | 120/135 | 111/135 |
| Anchoring | 63/63 | 54/63 | 52/63 | 50/63 |
| Character classes | 2535/2535 | 2526/2535 | 2282/2535 | 2452/2535 |
| Non-ASCII bytes | 16/16 | 16/16 | 10/16 | 9/16 |
| Nested .gitignore files | 68/68 | 68/68 | 67/68 | 3/3 † |
| Random scenarios (seed 1) | 5486/5486 | 5481/5486 | 5228/5486 | 1184/1265 † |
| **Total** | 9841/9841 | 9800/9841 | 9131/9841 | 5199/5555 † |

† excludes scenarios with nested .gitignore files, which this library can't load.
<!-- conformance:end -->

Beyond matching, go-ignore has:

- a filesystem walker that reads nested `.gitignore` files as it goes
- match provenance: the file and line of the deciding pattern
- no allocations per match for clean paths up to 32 segments (no `.`, `..` or `//`, lower case under `CaseInsensitive`)
- no dependencies outside the standard library

If you need `.dockerignore`/`.npmignore` (similar but not identical formats), full git repository access, or only trivial glob matching, a different tool may fit better — this library implements git's ignore spec and nothing more.

## Documentation

Full API reference, runnable examples, resource-limit tuning for untrusted input, thread-safety guarantees, and path-normalization rules are on **[pkg.go.dev](https://pkg.go.dev/github.com/Sriram-PR/go-ignore)**.

Key entry points:

- `New` / `NewWithOptions`: construct a matcher (options cover case sensitivity and resource limits).
- `LoadRepo`: load `core.excludesFile` (resolved as git does) + `.git/info/exclude` + root `.gitignore` in one call.
- `WalkDir` / `WalkRepo` / `Files` / `RepoFiles` / `WalkDirFS` / `FilesFS`: walk a tree (OS or `fs.FS`), skipping ignored files.
- `Entries` / `EntriesFS` / `RepoEntries`: iterate files and directories, with `SkipDir` and a `Start` subdirectory.
- `OpenTree` / `Tree.Check`: answer for one path as `git check-ignore` does, reading nested `.gitignore` files on demand.
- `AddPatterns` / `AddPatternsReader` / `AddPatternsFromFile` (plus `…WithSource` variants): add rules from bytes, a reader, or a file.
- `Match` / `MatchWithReason` / `Explain`: test a path, optionally with match provenance or the ignored parent that decided it.

## Stability Guarantees

Semantic versioning. Within the v1.x line:

- Exported identifiers keep their names and signatures. New ones may be added.
- The `Default*` limits keep their values. `MaxPathDepth` may be raised, never lowered. `MaxBacktrackIterations`, `DefaultMaxBacktrackIterations` and `HardMaxBacktrackIterations` are deprecated and ignored.
- Documented matching semantics stay the same, except bug fixes where the library disagreed with `git check-ignore`. Those ship in a patch or minor release with a note in [CHANGELOG.md](CHANGELOG.md).
- Unexported code and the wording of `ParseWarning.Message` are not part of the API.

## Contributing

Contributions are welcome; see [CONTRIBUTING.md](CONTRIBUTING.md). Run `make ci` and include tests for new behavior.

## License

MIT — see [LICENSE](LICENSE).
