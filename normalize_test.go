package ignore

import (
	"bytes"
	"runtime"
	"testing"
)

func TestNormalizePath(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		want        string
		windowsOnly bool // Skip on non-Windows (backslash is valid on Linux)
	}{
		// Basic cases
		{"empty string", "", "", false},
		{"simple path", "foo/bar", "foo/bar", false},
		{"single file", "file.txt", "file.txt", false},

		// Backslash conversion (Windows only — on Linux, \ is a valid filename char)
		{"windows backslash", "foo\\bar", "foo/bar", true},
		{"mixed slashes", "foo\\bar/baz", "foo/bar/baz", true},
		{"multiple backslashes", "foo\\bar\\baz", "foo/bar/baz", true},
		{"deep windows path", "a\\b\\c\\d\\e", "a/b/c/d/e", true},

		// Leading ./ removal
		{"leading dot slash", "./foo", "foo", false},
		{"leading dot slash nested", "./foo/bar", "foo/bar", false},
		{"dot slash only", "./", "", false},
		{"multiple leading dot slash", "././foo", "foo", false}, // All ./ removed for idempotency

		// Trailing slash removal
		{"trailing slash", "foo/", "foo", false},
		{"trailing slash nested", "foo/bar/", "foo/bar", false},
		{"only slash", "/", "", false},

		// Double slash collapse
		{"double slash", "foo//bar", "foo/bar", false},
		{"triple slash", "foo///bar", "foo/bar", false},
		{"multiple double slashes", "foo//bar//baz", "foo/bar/baz", false},
		{"leading double slash", "//foo", "/foo", false},

		// Combined cases (Windows only due to backslash conversion)
		{"windows with trailing", "foo\\bar\\", "foo/bar", true},
		{"dot slash with backslash", ".\\foo\\bar", "foo/bar", true},
		{"all combined", ".\\foo\\\\bar/baz//qux/", "foo/bar/baz/qux", true},

		// Edge cases
		{"just dot", ".", ".", false},
		{"dot dot", "..", "..", false},
		{"dot in middle", "foo/./bar", "foo/./bar", false}, // Only leading ./ is removed
		{"hidden file", ".gitignore", ".gitignore", false},
		{"hidden dir", ".git/config", ".git/config", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.windowsOnly && runtime.GOOS != "windows" {
				t.Skip("backslash conversion only applies on Windows")
			}
			got := normalizePath(tt.input)
			if got != tt.want {
				t.Errorf("normalizePath(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestNormalizeBasePath(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		want        string
		windowsOnly bool
	}{
		// Empty is repository root
		{"empty string", "", "", false},

		// Basic normalization
		{"simple path", "src", "src", false},
		{"nested path", "src/lib", "src/lib", false},

		// Trailing slash removed
		{"trailing slash", "src/", "src", false},

		// Backslash conversion (Windows only)
		{"windows path", "src\\lib", "src/lib", true},
		{"windows with trailing", "src\\lib\\", "src/lib", true},

		// Leading ./ removed
		{"leading dot slash", "./src", "src", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.windowsOnly && runtime.GOOS != "windows" {
				t.Skip("backslash conversion only applies on Windows")
			}
			got := normalizeBasePath(tt.input)
			if got != tt.want {
				t.Errorf("normalizeBasePath(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestNormalizeContent(t *testing.T) {
	tests := []struct {
		name  string
		input []byte
		want  []byte
	}{
		// Empty and nil
		{"empty", []byte{}, []byte{}},
		{"nil", nil, nil},

		// No changes needed
		{"simple content", []byte("*.log\nbuild/\n"), []byte("*.log\nbuild/\n")},
		{"no trailing newline", []byte("*.log"), []byte("*.log")},

		// UTF-8 BOM removal
		{"with BOM", []byte{0xEF, 0xBB, 0xBF, '*', '.', 'l', 'o', 'g'}, []byte("*.log")},
		{"BOM only", []byte{0xEF, 0xBB, 0xBF}, []byte{}},
		{"BOM with newlines", append([]byte{0xEF, 0xBB, 0xBF}, []byte("*.log\nbuild/")...), []byte("*.log\nbuild/")},
		{"double BOM", []byte{0xEF, 0xBB, 0xBF, 0xEF, 0xBB, 0xBF, '*', '.', 'l', 'o', 'g'}, []byte("*.log")},

		// CRLF normalization (Windows)
		{"CRLF endings", []byte("*.log\r\nbuild/\r\n"), []byte("*.log\nbuild/\n")},
		{"single CRLF", []byte("foo\r\nbar"), []byte("foo\nbar")},
		{"multiple CRLF", []byte("a\r\nb\r\nc\r\n"), []byte("a\nb\nc\n")},

		// CR only normalization (old Mac)
		{"CR only", []byte("*.log\rbuild/\r"), []byte("*.log\nbuild/\n")},
		{"single CR", []byte("foo\rbar"), []byte("foo\nbar")},

		// Mixed line endings
		{"mixed CRLF and LF", []byte("a\r\nb\nc\r\n"), []byte("a\nb\nc\n")},
		{"mixed all", []byte("a\r\nb\rc\n"), []byte("a\nb\nc\n")},

		// BOM with CRLF
		{"BOM and CRLF", append([]byte{0xEF, 0xBB, 0xBF}, []byte("*.log\r\nbuild/\r\n")...), []byte("*.log\nbuild/\n")},

		// Partial BOM (should not be removed)
		{"partial BOM 1 byte", []byte{0xEF, 'a', 'b'}, []byte{0xEF, 'a', 'b'}},
		{"partial BOM 2 bytes", []byte{0xEF, 0xBB, 'a'}, []byte{0xEF, 0xBB, 'a'}},

		// Content shorter than BOM
		{"1 byte", []byte{'a'}, []byte{'a'}},
		{"2 bytes", []byte{'a', 'b'}, []byte{'a', 'b'}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeContent(tt.input)
			if !bytes.Equal(got, tt.want) {
				t.Errorf("normalizeContent(%v) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestTrimTrailingWhitespace(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		// No whitespace
		{"no whitespace", "*.log", "*.log"},
		{"empty", "", ""},

		// Trailing spaces
		{"trailing space", "*.log ", "*.log"},
		{"multiple trailing spaces", "*.log   ", "*.log"},

		// Trailing tabs
		{"trailing tab", "*.log\t", "*.log"},
		{"multiple trailing tabs", "*.log\t\t", "*.log"},

		// Mixed trailing whitespace
		{"mixed space and tab", "*.log \t", "*.log"},
		{"mixed tab and space", "*.log\t ", "*.log"},

		// Leading whitespace preserved
		{"leading space", " *.log", " *.log"},
		{"leading and trailing", " *.log ", " *.log"},

		// Middle whitespace preserved
		{"middle space", "foo bar.txt", "foo bar.txt"},
		{"middle and trailing", "foo bar.txt  ", "foo bar.txt"},

		// Only whitespace
		{"only spaces", "   ", ""},
		{"only tabs", "\t\t", ""},

		// Backslash-escaped trailing spaces (git spec)
		{"escaped trailing space", "foo\\ ", "foo "},          // \<space> preserved, backslash removed
		{"escaped space then more", "foo\\   ", "foo "},       // \<space> preserved, extra spaces stripped
		{"double backslash space", "foo\\\\ ", "foo\\\\"},     // \\ = literal \, trailing space unescaped
		{"triple backslash space", "foo\\\\\\ ", "foo\\\\ "}, // \\\ = literal \ + escaped space
		{"backslash no space", "foo\\", "foo\\"},              // No trailing space, nothing to do
		{"backslash tab", "foo\\\t", "foo\\"},                 // Backslash before tab doesn't escape
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := trimTrailingWhitespace(tt.input)
			if got != tt.want {
				t.Errorf("trimTrailingWhitespace(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// TestNormalizePathIdempotent verifies that normalizing twice produces same result
func TestNormalizePathIdempotent(t *testing.T) {
	paths := []string{
		"foo/bar",
		"./foo",
		"foo/",
		"foo//bar",
		"././foo",
		"./././bar",
	}

	// Backslash paths are only meaningful on Windows
	if runtime.GOOS == "windows" {
		paths = append(paths, "foo\\bar", ".\\foo\\\\bar/")
	}

	for _, p := range paths {
		first := normalizePath(p)
		second := normalizePath(first)
		if first != second {
			t.Errorf("normalizePath not idempotent: normalizePath(%q) = %q, normalizePath(%q) = %q",
				p, first, first, second)
		}
	}
}

// TestNormalizeContentIdempotent verifies that normalizing twice produces same result
func TestNormalizeContentIdempotent(t *testing.T) {
	contents := [][]byte{
		[]byte("*.log\n"),
		[]byte("*.log\r\n"),
		{0xEF, 0xBB, 0xBF, '*', '.', 'l', 'o', 'g'},
		append([]byte{0xEF, 0xBB, 0xBF}, []byte("*.log\r\nbuild/\r\n")...),
		// Double BOM - this was the fuzz-discovered edge case
		{0xEF, 0xBB, 0xBF, 0xEF, 0xBB, 0xBF},
		{0xEF, 0xBB, 0xBF, 0xEF, 0xBB, 0xBF, 0xEF, 0xBB, 0xBF},
	}

	for i, c := range contents {
		first := normalizeContent(c)
		second := normalizeContent(first)
		if !bytes.Equal(first, second) {
			t.Errorf("normalizeContent not idempotent for case %d: first=%v, second=%v",
				i, first, second)
		}
	}
}
