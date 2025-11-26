package ignore

import (
	"testing"
)

// FuzzAddPatterns fuzzes the pattern parsing
func FuzzAddPatterns(f *testing.F) {
	// Seed corpus with interesting patterns
	seeds := [][]byte{
		[]byte("*.log"),
		[]byte("build/"),
		[]byte("!important.log"),
		[]byte("**/temp"),
		[]byte("a/**/b"),
		[]byte("foo/**"),
		[]byte("#comment"),
		[]byte(""),
		[]byte("   "),
		[]byte("\n\n\n"),
		[]byte("*.log\nbuild/\n"),
		[]byte("!\n"),
		[]byte("/\n"),
		[]byte("\\#notcomment"),
		[]byte("file with spaces.txt"),
		[]byte("日本語.txt"),
		[]byte("*.tar.gz"),
		[]byte("*test*.go"),
		// BOM
		{0xEF, 0xBB, 0xBF, '*', '.', 'l', 'o', 'g'},
		// CRLF
		[]byte("*.log\r\nbuild/\r\n"),
		// CR only
		[]byte("*.log\rbuild/\r"),
		// Mixed
		[]byte("*.log\r\n!important.log\nbuild/\r"),
	}

	for _, seed := range seeds {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, content []byte) {
		m := New()

		// Should never panic
		warnings := m.AddPatterns("", content)
		_ = warnings

		// Warnings should return without panic
		_ = m.Warnings()

		// RuleCount should work
		_ = m.RuleCount()

		// Multiple AddPatterns calls should work
		m.AddPatterns("src", content)
		m.AddPatterns("src/lib", content)
	})
}

// FuzzMatch fuzzes the matching logic
func FuzzMatch(f *testing.F) {
	// Seed corpus with interesting paths
	seeds := []string{
		"file.txt",
		"src/main.go",
		"build/output.js",
		"node_modules/lodash/index.js",
		"a/b/c/d/e/f/g/h.txt",
		".hidden",
		".git/config",
		"file with spaces.txt",
		"日本語.txt",
		"",
		".",
		"..",
		"/",
		"//",
		"a//b",
		"./src/main.go",
		"src\\main.go",
		"path/to/file.log",
	}

	for _, seed := range seeds {
		f.Add(seed, false)
		f.Add(seed, true)
	}

	// Create a matcher with various patterns
	m := New()
	m.AddPatterns("", []byte(`
*.log
*.tmp
build/
!important.log
**/cache/
src/**/test/
.hidden
node_modules/
*.tar.gz
`))

	f.Fuzz(func(t *testing.T, path string, isDir bool) {
		// Should never panic
		_ = m.Match(path, isDir)
		_ = m.MatchWithReason(path, isDir)
	})
}

// FuzzPatternAndPath fuzzes both pattern and path together
func FuzzPatternAndPath(f *testing.F) {
	// Seed with pattern, path pairs
	seeds := []struct {
		pattern string
		path    string
	}{
		{"*.log", "test.log"},
		{"build/", "build/output.js"},
		{"**/temp", "a/b/temp"},
		{"!important.log", "important.log"},
		{"src/**/test", "src/lib/test"},
		{"*.tar.gz", "archive.tar.gz"},
		{"*test*", "mytest.go"},
		{"a/**/b/**/c", "a/x/b/y/c"},
	}

	for _, seed := range seeds {
		f.Add(seed.pattern, seed.path, false)
		f.Add(seed.pattern, seed.path, true)
	}

	f.Fuzz(func(t *testing.T, pattern, path string, isDir bool) {
		m := New()

		// Should never panic even with arbitrary patterns
		m.AddPatterns("", []byte(pattern+"\n"))
		_ = m.Match(path, isDir)
		_ = m.MatchWithReason(path, isDir)
	})
}

// FuzzGlob fuzzes the glob matching function
func FuzzGlob(f *testing.F) {
	seeds := []struct {
		pattern string
		s       string
	}{
		{"*", "anything"},
		{"*.log", "test.log"},
		{"test_*", "test_foo"},
		{"*_test", "foo_test"},
		{"*a*b*c*", "xaybzc"},
		{"", ""},
		{"*", ""},
		{"**", "test"},
		{"***", "test"},
	}

	for _, seed := range seeds {
		f.Add(seed.pattern, seed.s)
	}

	f.Fuzz(func(t *testing.T, pattern, s string) {
		// Should never panic
		_ = matchGlob(pattern, s)
	})
}

// FuzzNormalizePath fuzzes path normalization
func FuzzNormalizePath(f *testing.F) {
	seeds := []string{
		"src/main.go",
		"src\\main.go",
		"./src/main.go",
		"src//main.go",
		"",
		"/",
		"\\",
		"./",
		".\\",
		"//",
		"\\\\",
		"a/b/c",
		"a\\b\\c",
		"./a/./b/./c",
	}

	for _, seed := range seeds {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, path string) {
		// Should never panic
		result := normalizePath(path)

		// Result should be idempotent
		result2 := normalizePath(result)
		if result != result2 {
			t.Errorf("normalizePath not idempotent: %q -> %q -> %q", path, result, result2)
		}

		// Result should not contain backslashes
		for i := 0; i < len(result); i++ {
			if result[i] == '\\' {
				t.Errorf("result contains backslash at position %d", i)
			}
		}

		// Result should not contain double slashes
		if len(result) > 1 {
			for i := 0; i < len(result)-1; i++ {
				if result[i] == '/' && result[i+1] == '/' {
					t.Errorf("result contains double slash at position %d", i)
				}
			}
		}

		// Result should not have leading ./ (unless the original was just ".")
		if len(result) >= 2 && result[0] == '.' && result[1] == '/' {
			t.Errorf("result has leading ./: %q", result)
		}

		// Result should not have trailing slash (unless empty)
		if len(result) > 0 && result[len(result)-1] == '/' {
			t.Errorf("result has trailing slash: %q", result)
		}
	})
}

// FuzzNormalizeContent fuzzes content normalization
func FuzzNormalizeContent(f *testing.F) {
	seeds := [][]byte{
		[]byte("test"),
		[]byte("test\n"),
		[]byte("test\r\n"),
		[]byte("test\r"),
		{0xEF, 0xBB, 0xBF, 't', 'e', 's', 't'},
		[]byte("line1\r\nline2\nline3\rline4"),
		{},
		nil,
	}

	for _, seed := range seeds {
		if seed != nil {
			f.Add(seed)
		}
	}

	f.Fuzz(func(t *testing.T, content []byte) {
		// Should never panic
		result := normalizeContent(content)

		// Result should be idempotent
		result2 := normalizeContent(result)
		if string(result) != string(result2) {
			t.Errorf("normalizeContent not idempotent")
		}

		// Result should not contain CRLF or CR
		for i := 0; i < len(result); i++ {
			if result[i] == '\r' {
				t.Errorf("result contains CR at position %d", i)
			}
		}
	})
}

// FuzzMatchSegments fuzzes segment matching
func FuzzMatchSegments(f *testing.F) {
	// Add seeds for pattern and path combinations
	f.Add("foo", "foo")
	f.Add("foo/bar", "foo/bar")
	f.Add("*/bar", "foo/bar")
	f.Add("**/bar", "foo/bar")
	f.Add("foo/**", "foo/bar")
	f.Add("a/**/b", "a/x/y/z/b")

	f.Fuzz(func(t *testing.T, pattern, path string) {
		// Parse pattern into segments
		segments := parseSegments(pattern)
		pathSegs := splitPath(path)

		// Should never panic
		ctx := newMatchContext(1000) // Limit iterations for fuzzing
		_ = matchSegments_(segments, pathSegs, ctx, false)
	})
}

// FuzzConcurrentAccess fuzzes concurrent matcher access
func FuzzConcurrentAccess(f *testing.F) {
	f.Add([]byte("*.log\nbuild/\n"), "test.log", false)

	f.Fuzz(func(t *testing.T, content []byte, path string, isDir bool) {
		m := New()
		m.AddPatterns("", content)

		// Run concurrent operations
		done := make(chan bool, 10)

		for i := 0; i < 5; i++ {
			go func() {
				m.Match(path, isDir)
				done <- true
			}()
			go func() {
				m.MatchWithReason(path, isDir)
				done <- true
			}()
		}

		// Wait for all goroutines
		for i := 0; i < 10; i++ {
			<-done
		}
	})
}