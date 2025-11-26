package ignore

import (
	"testing"
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

		// Double dots
		{"dotdot pattern", "..", "..", true, false},

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
			"nested file negation",
			"logs/\n!logs/keep.log",
			"logs/keep.log",
			false,
		},
		{
			"nested dir negation",
			"temp/\n!temp/important/",
			"temp/important",
			false,
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
	warnings := m.AddPatterns("", []byte{})
	if len(warnings) != 0 {
		t.Errorf("empty content should produce no warnings")
	}

	// Nil content
	warnings = m.AddPatterns("", nil)
	if warnings != nil {
		t.Errorf("nil content should return nil warnings")
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
		name     string
		basePath string
		pattern  string
		path     string
		want     bool
	}{
		{"basePath with trailing slash", "src/", "*.log", "src/test.log", true},
		{"basePath with backslash", "src\\lib", "*.log", "src/lib/test.log", true},
		{"basePath with leading dot-slash", "./src", "*.log", "src/test.log", true},
		{"empty basePath", "", "*.log", "test.log", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
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
