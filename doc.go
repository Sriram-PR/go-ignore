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
//   - Question mark: "?.txt" matches any single-character name
//   - Double star: "**/logs" matches at any depth
//   - Negation: "!important.log" re-includes a file
//   - Character classes: "[abc]" matches one of a, b, or c
//   - Ranges: "[a-z]", "[0-9]" match character ranges
//   - Negated classes: "[!abc]" matches any character except a, b, or c
//   - POSIX classes: "[[:alpha:]]", "[[:digit:]]" and 10 more
//   - Escapes: "\*", "\?", "\#", "\!" for literal matching
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
