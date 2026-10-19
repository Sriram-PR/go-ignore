// Package ignore provides gitignore pattern matching for file paths.
//
// It is a minimal, zero-dependency implementation of the gitignore spec,
// verified against git check-ignore. It supports the full pattern syntax:
// wildcards (*), single character (?), globstar (**), negation (!),
// directory-only (trailing /), anchoring (leading /), escapes (\), and
// character classes including all 12 POSIX classes.
//
// # Basic usage
//
//	m := ignore.New()
//	m.AddPatterns("", []byte("*.log\nbuild/\n!important.log\n"))
//	m.Match("debug.log", false)     // true
//	m.Match("important.log", false) // false (negated)
//
// The second Match argument reports whether the path is a directory; it must
// be accurate for directory-only patterns. The library does not auto-ignore
// .git/ — add ".git/" yourself if you want git-like walking.
//
// # Loading and walking
//
// LoadRepo pre-loads the standard gitignore sources for a working tree:
// core.excludesFile as git resolves it, .git/info/exclude, and the root
// .gitignore. WalkDir and WalkRepo walk a tree, auto-discovering nested
// .gitignore files and pruning ignored (and .git/) directories; Files and
// RepoFiles are their range-over-func form, and WalkDirFS/FilesFS operate
// over an fs.FS. See each function's own documentation for details.
//
// # Match provenance
//
// MatchWithReason reports which rule decided a path — including its source file
// and line — for debugging complex setups.
//
// # Syntax notes
//
// ? and character classes operate on raw bytes, not Unicode code points
// (matching git), so a multi-byte character needs multiple ?. Input paths are
// normalized (slashes, leading "./", trailing "/", and ".." resolved so a path
// cannot bypass a scoped rule); patterns keep their original escapes.
//
// # Thread safety
//
// A Matcher is safe for concurrent use: many goroutines may call Match while
// AddPatterns runs concurrently. For best throughput, batch AddPatterns calls
// before starting concurrent Match calls.
//
// # Resource limits
//
// MatcherOptions bounds the pattern count and line length for safe handling
// of untrusted input; see MatcherOptions and the Default* constants. Matching
// itself needs no limit: it costs polynomial time for any pattern and path.
package ignore
