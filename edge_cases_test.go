package ignore

import (
	"runtime"
	"strings"
	"testing"
	"time"
)

// TestEdgeCases_LineEndings tests various line ending formats
func TestEdgeCases_LineEndings(t *testing.T) {
	tests := []struct {
		name    string
		content []byte
		path    string
		want    bool
		isDir   bool
	}{
		// CRLF (Windows)
		{
			"CRLF line endings",
			[]byte("*.log\r\nbuild/\r\n"),
			"test.log",
			true,
			false,
		},
		{
			"CRLF second pattern dir",
			[]byte("*.log\r\nbuild/\r\n"),
			"build",
			true,
			true, // directory pattern needs isDir=true
		},
		{
			"CRLF second pattern file inside",
			[]byte("*.log\r\nbuild/\r\n"),
			"build/output.js",
			true,
			false, // file inside directory
		},
		// CR only (old Mac)
		{
			"CR only line endings",
			[]byte("*.log\rbuild/\r"),
			"test.log",
			true,
			false,
		},
		// Mixed line endings
		{
			"mixed CRLF and LF",
			[]byte("*.log\r\n*.tmp\nbuild/\r\n"),
			"test.tmp",
			true,
			false,
		},
		// No trailing newline
		{
			"no trailing newline",
			[]byte("*.log"),
			"test.log",
			true,
			false,
		},
		// Multiple blank lines
		{
			"multiple blank lines",
			[]byte("*.log\n\n\n\nbuild/"),
			"test.log",
			true,
			false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := New()
			m.AddPatterns("", tt.content)
			got := m.Match(tt.path, tt.isDir)
			if got != tt.want {
				t.Errorf("Match(%q, isDir=%v) = %v, want %v", tt.path, tt.isDir, got, tt.want)
			}
		})
	}
}

// TestEdgeCases_BOM tests UTF-8 BOM handling
func TestEdgeCases_BOM(t *testing.T) {
	bom := []byte{0xEF, 0xBB, 0xBF}

	tests := []struct {
		name    string
		content []byte
		path    string
		want    bool
		isDir   bool
	}{
		{
			"BOM at start",
			append(bom, []byte("*.log\n")...),
			"test.log",
			true,
			false,
		},
		{
			"BOM with CRLF",
			append(bom, []byte("*.log\r\nbuild/\r\n")...),
			"test.log",
			true,
			false,
		},
		{
			"BOM only content dir",
			append(bom, []byte("build/")...),
			"build",
			true,
			true, // directory pattern needs isDir=true for exact match
		},
		{
			"BOM only content file inside dir",
			append(bom, []byte("build/")...),
			"build/output.js",
			true,
			false, // file inside directory
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := New()
			m.AddPatterns("", tt.content)
			got := m.Match(tt.path, tt.isDir)
			if got != tt.want {
				t.Errorf("Match(%q, isDir=%v) = %v, want %v", tt.path, tt.isDir, got, tt.want)
			}
		})
	}
}

// TestEdgeCases_Unicode tests Unicode filename handling
func TestEdgeCases_Unicode(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		path    string
		want    bool
	}{
		// Japanese
		{"japanese filename", "日本語.txt", "日本語.txt", true},
		{"japanese wildcard", "*.日本語", "test.日本語", true},
		{"japanese directory", "日本語/", "日本語/file.txt", true},

		// Chinese
		{"chinese filename", "中文.txt", "中文.txt", true},
		{"chinese path", "文档/", "文档/readme.md", true},

		// Emoji
		{"emoji filename", "🎉.txt", "🎉.txt", true},
		{"emoji wildcard", "*.🎉", "party.🎉", true},

		// Accented characters
		{"french accents", "café.txt", "café.txt", true},
		{"german umlaut", "über.txt", "über.txt", true},
		{"spanish tilde", "año.txt", "año.txt", true},

		// Mixed scripts
		{"mixed unicode", "test_日本語_data.txt", "test_日本語_data.txt", true},

		// Unicode in directory patterns
		{"unicode dir pattern", "données/", "données/file.csv", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := New()
			m.AddPatterns("", []byte(tt.pattern+"\n"))
			// For directory patterns, test with isDir=true for exact match
			isDir := false
			if len(tt.pattern) > 0 && tt.pattern[len(tt.pattern)-1] == '/' {
				// If path doesn't contain /, it's the dir itself
				if tt.path == tt.pattern[:len(tt.pattern)-1] {
					isDir = true
				}
			}
			got := m.Match(tt.path, isDir)
			if got != tt.want {
				t.Errorf("Match(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

// TestEdgeCases_Whitespace tests whitespace handling in patterns
func TestEdgeCases_Whitespace(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		path    string
		want    bool
	}{
		// Trailing whitespace (should be trimmed)
		{"trailing spaces trimmed", "*.log   ", "test.log", true},
		{"trailing tabs trimmed", "*.log\t\t", "test.log", true},

		// Spaces in filenames (valid)
		{"space in filename", "foo bar.txt", "foo bar.txt", true},
		{"space in pattern", "my file.log", "my file.log", true},
		{"space in directory", "my dir/", "my dir/file.txt", true},

		// Leading spaces (preserved in gitignore)
		{"leading space", " leading.txt", " leading.txt", true},

		// Multiple spaces in middle
		{"multiple spaces", "foo  bar.txt", "foo  bar.txt", true},

		// Backslash-escaped trailing space: the literal pattern is "foo " (with one space).
		// Per spec, "\ " at the end preserves a trailing space that would otherwise be trimmed.
		{"escaped trailing space matches space", `foo\ `, "foo ", true},
		{"escaped trailing space does not match unspaced", `foo\ `, "foo", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := New()
			m.AddPatterns("", []byte(tt.pattern+"\n"))
			got := m.Match(tt.path, false)
			if got != tt.want {
				t.Errorf("Match(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

// TestEdgeCases_EscapedBackslash covers pattern "foo\\" — the gitignore escape
// for a literal backslash in a filename. This scenario is Unix-only: on
// Windows, backslash is the path separator and gets converted to '/' during
// normalization, so a filename containing a literal '\' is unrepresentable.
func TestEdgeCases_EscapedBackslash(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("backslash is a path separator on Windows; literal-backslash filenames are not representable")
	}

	tests := []struct {
		name    string
		pattern string
		path    string
		want    bool
	}{
		{"escaped backslash matches literal backslash", `foo\\`, `foo\`, true},
		{"escaped backslash does not match unbackslashed", `foo\\`, "foo", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := New()
			m.AddPatterns("", []byte(tt.pattern+"\n"))
			got := m.Match(tt.path, false)
			if got != tt.want {
				t.Errorf("Match(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

// TestEdgeCases_SpecialPatterns tests edge cases in pattern syntax
func TestEdgeCases_SpecialPatterns(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		path    string
		want    bool
		isDir   bool
	}{
		// Dot files
		{"hidden file", ".hidden", ".hidden", true, false},
		{"hidden file nested", ".hidden", "src/.hidden", true, false},
		{"hidden directory", ".cache/", ".cache", true, true},
		{"hidden directory contents", ".cache/", ".cache/data.bin", true, false},

		// Double dots — ".." resolves above repo root, so it's treated as empty (no match)
		{"dotdot pattern", "..", "..", false, false},

		// Single character
		{"single char file", "a", "a", true, false},
		{"single char nested", "a", "dir/a", true, false},

		// Numeric names
		{"numeric file", "123", "123", true, false},
		{"numeric with extension", "123.txt", "123.txt", true, false},

		// Stars in various positions
		{"star only", "*", "anything", true, false},
		{"double star only", "**", "a/b/c", true, false},
		{"triple star", "***", "file", true, false}, // treated as wildcard

		// Consecutive slashes in pattern (normalized)
		{"double slash pattern", "a//b", "a/b", true, false},

		// Pattern with dots
		{"extension dots", "*.tar.gz", "archive.tar.gz", true, true},
		{"multiple dots", "file.test.spec.ts", "file.test.spec.ts", true, false},

		// Wildcards at different positions
		{"wildcard prefix", "*_test.go", "foo_test.go", true, false},
		{"wildcard suffix", "test_*", "test_foo", true, false},
		{"wildcard both", "*test*", "mytestfile", true, false},
		{"wildcard middle", "a*b", "aXXXb", true, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := New()
			m.AddPatterns("", []byte(tt.pattern+"\n"))
			got := m.Match(tt.path, tt.isDir)
			if got != tt.want {
				t.Errorf("pattern %q, Match(%q, %v) = %v, want %v",
					tt.pattern, tt.path, tt.isDir, got, tt.want)
			}
		})
	}
}

// TestEdgeCases_Negation tests complex negation scenarios
func TestEdgeCases_Negation(t *testing.T) {
	tests := []struct {
		name     string
		patterns string
		path     string
		want     bool
	}{
		// Basic negation
		{
			"simple negation",
			"*.log\n!important.log",
			"important.log",
			false,
		},
		// Re-ignore after negation
		{
			"re-ignore after negation",
			"*.log\n!important.log\nimportant.log",
			"important.log",
			true,
		},
		// Negation without prior match (no effect)
		{
			"negation without match",
			"!foo.txt",
			"foo.txt",
			false,
		},
		// Multiple negations
		{
			"multiple negations",
			"*\n!*.go\n!*.md",
			"readme.md",
			false,
		},
		{
			"multiple negations other file",
			"*\n!*.go\n!*.md",
			"main.go",
			false,
		},
		{
			"multiple negations ignored file",
			"*\n!*.go\n!*.md",
			"config.json",
			true,
		},
		// Directory negation
		{
			"directory negation",
			"build/\n!build/",
			"build",
			false,
		},
		// Nested negation
		{
			// Spec: parent dir excluded blocks re-include via negation.
			// git check-ignore agrees: logs/keep.log is ignored by logs/.
			"nested file negation blocked by parent",
			"logs/\n!logs/keep.log",
			"logs/keep.log",
			true,
		},
		{
			// Spec: parent dir excluded blocks re-include via negation.
			// git check-ignore agrees: temp/important is ignored.
			"nested dir negation blocked by parent",
			"temp/\n!temp/important/",
			"temp/important",
			true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := New()
			m.AddPatterns("", []byte(tt.patterns))
			// Check if path looks like a directory (for this test)
			isDir := tt.path == "build" || tt.path == "temp/important"
			got := m.Match(tt.path, isDir)
			if got != tt.want {
				t.Errorf("Match(%q) = %v, want %v\npatterns:\n%s",
					tt.path, got, tt.want, tt.patterns)
			}
		})
	}
}

// TestEdgeCases_PathVariations tests various path formats
func TestEdgeCases_PathVariations(t *testing.T) {
	m := New()
	m.AddPatterns("", []byte("*.log\nbuild/\nsrc/temp/\n"))

	tests := []struct {
		name  string
		path  string
		isDir bool
		want  bool
	}{
		// Windows paths
		// Note: On Linux, backslash is a literal filename character, so
		// "src\lib\test.log" is treated as a single path segment. The *.log
		// pattern still matches because it floats and glob-matches the segment.
		{"windows backslash", "test.log", false, true},
		{"windows deep path", "src\\lib\\test.log", false, true},
		{"windows dir", "build", true, true},

		// Leading ./
		{"leading dot slash", "./test.log", false, true},
		{"leading dot slash nested", "./src/test.log", false, true},

		// Trailing /
		{"trailing slash file", "test.log/", false, true}, // normalized

		// Double slashes
		{"double slash", "src//test.log", false, true},

		// Mixed
		{"mixed slashes", "src\\lib//test.log", false, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := m.Match(tt.path, tt.isDir)
			if got != tt.want {
				t.Errorf("Match(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

// TestEdgeCases_EmptyAndNil tests empty/nil input handling
func TestEdgeCases_EmptyAndNil(t *testing.T) {
	m := New()

	// Empty content
	m.AddPatterns("", []byte{})
	if w := m.Warnings(); len(w) != 0 {
		t.Errorf("empty content should produce no warnings")
	}

	// Nil content
	m.AddPatterns("", nil)
	if w := m.Warnings(); len(w) != 0 {
		t.Errorf("nil content should produce no warnings")
	}

	// Add actual pattern
	m.AddPatterns("", []byte("*.log\n"))

	// Empty path
	if m.Match("", false) {
		t.Error("empty path should not match")
	}

	// Whitespace-only path (normalized to empty)
	// Note: This is edge case behavior
}

// TestEdgeCases_VeryLongPaths tests handling of very long paths
func TestEdgeCases_VeryLongPaths(t *testing.T) {
	m := New()
	m.AddPatterns("", []byte("**/deep.txt\n*.log\n"))

	// Create a very deep path
	deepPath := ""
	for i := 0; i < 50; i++ {
		deepPath += "dir/"
	}
	deepPath += "deep.txt"

	if !m.Match(deepPath, false) {
		t.Error("should match very deep path with **")
	}

	// Very long filename
	longName := ""
	for i := 0; i < 200; i++ {
		longName += "a"
	}
	longName += ".log"

	if !m.Match(longName, false) {
		t.Error("should match very long filename")
	}
}

// TestEdgeCases_DeepPath_BoundedByDepthCap guards against quadratic blowup on
// extremely deep paths. Two issues compounded in CI's FuzzMatch:
//
//   - the parent-excluded ancestor walk did strings.Join on growing prefix
//     slices, giving O(N²) string allocations;
//   - for every ancestor, evaluateRules re-ran every unanchored rule across
//     all ancestor segments, giving O(M·N²) matching work.
//
// The fix combines two changes: the ancestor walk now slices `path` at slash
// positions instead of joining segments, and a defensive depth cap
// (MaxPathDepth, default 4096) makes pathological inputs short-circuit
// rather than exhibit quadratic behavior. This test asserts the cap fires
// on a deep path and that legitimately-deep paths under the cap still
// complete in well under a second.
func TestEdgeCases_DeepPath_BoundedByDepthCap(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping deep-path perf guard in -short mode")
	}
	m := New()
	m.AddPatterns("", []byte(`
*.log
build/
!important.log
node_modules/
`))

	// Above the cap: must short-circuit (Match returns false, no walk).
	tooDeep := buildDeepPath(50_000, "important.log")
	deadline := time.NewTimer(500 * time.Millisecond)
	defer deadline.Stop()
	done := make(chan bool)
	go func() {
		done <- m.Match(tooDeep, false)
	}()
	select {
	case got := <-done:
		if got {
			t.Errorf("path past depth cap should return Match=false (short-circuit), got true")
		}
	case <-deadline.C:
		t.Fatalf("Match on path past depth cap did not return within 500ms")
	}

	// Just under the cap: must complete in a reasonable time even when
	// the ancestor walk runs in full.
	underCap := buildDeepPath(1000, "important.log")
	deadline2 := time.NewTimer(1 * time.Second)
	defer deadline2.Stop()
	done2 := make(chan struct{})
	go func() {
		_ = m.Match(underCap, false)
		close(done2)
	}()
	select {
	case <-done2:
	case <-deadline2.C:
		t.Fatalf("Match on 1000-segment path did not return within 1s — ancestor walk regression")
	}
}

func buildDeepPath(n int, leaf string) string {
	var b strings.Builder
	b.Grow(n*2 + len(leaf))
	for i := 0; i < n; i++ {
		b.WriteString("a/")
	}
	b.WriteString(leaf)
	return b.String()
}

// TestEdgeCases_ManyPatterns tests handling of many patterns
func TestEdgeCases_ManyPatterns(t *testing.T) {
	m := New()

	// Add 1000 patterns
	content := ""
	for i := 0; i < 1000; i++ {
		content += "*.ext" + string(rune('0'+i%10)) + "\n"
	}
	content += "target.txt\n"

	m.AddPatterns("", []byte(content))

	if m.RuleCount() != 1001 {
		t.Errorf("RuleCount = %d, want 1001", m.RuleCount())
	}

	// Should still match
	if !m.Match("target.txt", false) {
		t.Error("should match target.txt with many patterns")
	}
}

// TestEdgeCases_RuleOrder tests that rule order is preserved
func TestEdgeCases_RuleOrder(t *testing.T) {
	m := New()
	m.AddPatterns("", []byte("*.log\n!important.log\n*.log\n"))

	// Last *.log should win
	result := m.MatchWithReason("important.log", false)
	if !result.Ignored {
		t.Error("last *.log should override negation")
	}
	if result.Line != 3 {
		t.Errorf("Line = %d, want 3 (last matching rule)", result.Line)
	}
}

// TestEdgeCases_BasePathNormalization tests basePath handling
func TestEdgeCases_BasePathNormalization(t *testing.T) {
	tests := []struct {
		name        string
		basePath    string
		pattern     string
		path        string
		want        bool
		windowsOnly bool
	}{
		{"basePath with trailing slash", "src/", "*.log", "src/test.log", true, false},
		{"basePath with backslash", "src\\lib", "*.log", "src/lib/test.log", true, true},
		{"basePath with leading dot-slash", "./src", "*.log", "src/test.log", true, false},
		{"empty basePath", "", "*.log", "test.log", true, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.windowsOnly && runtime.GOOS != "windows" {
				t.Skip("backslash conversion only applies on Windows")
			}
			m := New()
			m.AddPatterns(tt.basePath, []byte(tt.pattern+"\n"))
			got := m.Match(tt.path, false)
			if got != tt.want {
				t.Errorf("basePath=%q, pattern=%q, Match(%q) = %v, want %v",
					tt.basePath, tt.pattern, tt.path, got, tt.want)
			}
		})
	}
}
