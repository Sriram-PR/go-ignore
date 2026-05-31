// Package ignore provides gitignore pattern matching for file paths.
//
// This is a minimal, zero-dependency library for matching file paths against
// .gitignore patterns. It supports the common gitignore syntax including
// wildcards (*), double-star (**), negation (!), and directory-only patterns.
//
// # Basic Usage
//
//	m := ignore.New()
//
//	// Add .git/ explicitly if you want Git-like behavior
//	m.AddPatterns("", []byte(".git/\n"))
//
//	// Load .gitignore content
//	content, _ := os.ReadFile(".gitignore")
//	m.AddPatterns("", content)
//
//	// Check if a path should be ignored
//	if m.Match("node_modules/foo.js", false) {
//	    // path is ignored
//	}
//
// # Nested .gitignore Files
//
// For repositories with nested .gitignore files, specify the base path:
//
//	// Root .gitignore
//	m.AddPatterns("", rootContent)
//
//	// Nested src/.gitignore
//	m.AddPatterns("src", srcContent)
//
// # Thread Safety
//
// Matcher is safe for concurrent use. Multiple goroutines can call Match
// simultaneously. AddPatterns can also be called concurrently, though for
// best performance, batch all AddPatterns calls before concurrent Match calls.
//
// # Supported Syntax
//
// The following gitignore patterns are supported:
//
//   - Plain names: "debug.log" matches anywhere in tree
//   - Leading /: "/debug.log" matches only at base path
//   - Trailing /: "build/" matches directories only
//   - Single star: "*.log" matches any .log file
//   - Question mark: "?.txt" matches any single byte (not Unicode code point)
//   - Double star: "**/logs" matches at any depth
//   - Negation: "!important.log" re-includes a file
//   - Character classes: "[abc]" matches one byte: a, b, or c
//   - Ranges: "[a-z]", "[0-9]" match character ranges
//   - Negated classes: "[!abc]" or "[^abc]" matches any character except a, b, or c
//   - POSIX classes: "[[:alpha:]]", "[[:digit:]]" and 10 more
//   - Escapes: "\*", "\?", "\#", "\!" for literal matching
//
// Note: ? and [...] operate on raw bytes, not Unicode code points,
// consistent with Git's behavior.
//
// The backtrack iteration budget (MaxBacktrackIterations, default 10,000)
// is shared across all rules within a single Match call. This prevents
// pathological patterns distributed across many rules from causing
// excessive CPU usage.
//
// # Global Gitignore
//
// Load the user's global gitignore file (core.excludesFile or
// ~/.config/git/ignore) with a single call:
//
//	m := ignore.New()
//	if err := m.AddGlobalPatterns(); err != nil {
//	    log.Fatal(err)
//	}
//
// # Repository Exclude File
//
// Load the repository's .git/info/exclude file:
//
//	if err := m.AddExcludePatterns(".git"); err != nil {
//	    log.Fatal(err)
//	}
//
// # Loading a Working Tree in One Call
//
// LoadRepo is a convenience constructor that pre-loads the four standard
// gitignore sources (system, global, .git/info/exclude, root .gitignore) for
// a working tree in git's precedence order:
//
//	m, err := ignore.LoadRepo(".", ignore.MatcherOptions{})
//	if err != nil {
//	    log.Fatal(err)
//	}
//	m.Match("build/output.js", false)
//
// Paths passed to Match must be relative to repoRoot. LoadRepo does NOT walk
// nested per-directory .gitignore files; use WalkDir / WalkRepo for that.
//
// # Walking a Working Tree
//
// WalkDir and WalkRepo walk a directory tree, applying nested .gitignore
// discovery automatically and skipping ignored entries before calling the
// user callback. The .git/ directory is always pruned.
//
//	err := ignore.WalkRepo(".", ignore.MatcherOptions{}, func(path string, d fs.DirEntry, err error) error {
//	    if err != nil { return err }
//	    if !d.IsDir() { process(path) }
//	    return nil
//	})
//
// WalkDir does not mutate its receiver: discovered nested rules live only
// for the duration of the call.
//
// For range-over-func ergonomics, Files (method) and RepoFiles (standalone)
// yield only non-ignored files as an iter.Seq2[string, error]. For walking
// an fs.FS (fstest.MapFS, embed.FS, etc.) instead of the OS filesystem,
// use WalkDirFS.
//
// # Streaming Patterns from an io.Reader
//
// AddPatternsReader accepts an io.Reader so callers do not need to read the
// whole file into a byte slice first:
//
//	f, _ := os.Open(".gitignore")
//	defer f.Close()
//	if err := m.AddPatternsReader("", f); err != nil {
//	    log.Fatal(err)
//	}
//
// # Path Normalization
//
// Input paths are automatically normalized:
//
//   - Backslashes converted to forward slashes (Windows only)
//   - Leading ./ removed
//   - Trailing / removed
//   - Consecutive slashes collapsed
//
// On Windows, backslash paths work transparently:
//
//	m.Match("src\\main.go", false)  // works on Windows
//
// On Linux/macOS, backslashes are valid filename characters and are not
// converted. Always use forward slashes for portable code.
package ignore
