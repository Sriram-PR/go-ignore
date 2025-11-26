package ignore

import (
	"bytes"
	"testing"
)

func TestNormalizePath(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		// Basic cases
		{"empty string", "", ""},
		{"simple path", "foo/bar", "foo/bar"},
		{"single file", "file.txt", "file.txt"},

		// Backslash conversion (Windows)
		{"windows backslash", "foo\\bar", "foo/bar"},
		{"mixed slashes", "foo\\bar/baz", "foo/bar/baz"},
		{"multiple backslashes", "foo\\bar\\baz", "foo/bar/baz"},
		{"deep windows path", "a\\b\\c\\d\\e", "a/b/c/d/e"},

		// Leading ./ removal
		{"leading dot slash", "./foo", "foo"},
		{"leading dot slash nested", "./foo/bar", "foo/bar"},
		{"dot slash only", "./", ""},
		{"multiple leading dot slash", "././foo", "foo"}, // All ./ removed for idempotency

		// Trailing slash removal
		{"trailing slash", "foo/", "foo"},
		{"trailing slash nested", "foo/bar/", "foo/bar"},
		{"only slash", "/", ""},

		// Double slash collapse
		{"double slash", "foo//bar", "foo/bar"},
		{"triple slash", "foo///bar", "foo/bar"},
		{"multiple double slashes", "foo//bar//baz", "foo/bar/baz"},
		{"leading double slash", "//foo", "/foo"},

		// Combined cases
		{"windows with trailing", "foo\\bar\\", "foo/bar"},
		{"dot slash with backslash", ".\\foo\\bar", "foo/bar"},
		{"all combined", ".\\foo\\\\bar/baz//qux/", "foo/bar/baz/qux"},

		// Edge cases
		{"just dot", ".", "."},
		{"dot dot", "..", ".."},
		{"dot in middle", "foo/./bar", "foo/./bar"}, // Only leading ./ is removed
		{"hidden file", ".gitignore", ".gitignore"},
		{"hidden dir", ".git/config", ".git/config"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizePath(tt.input)
			if got != tt.want {
				t.Errorf("normalizePath(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestNormalizeBasePath(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		// Empty is repository root
		{"empty string", "", ""},

		// Basic normalization
		{"simple path", "src", "src"},
		{"nested path", "src/lib", "src/lib"},

		// Trailing slash removed
		{"trailing slash", "src/", "src"},

		// Backslash conversion
		{"windows path", "src\\lib", "src/lib"},
		{"windows with trailing", "src\\lib\\", "src/lib"},

		// Leading ./ removed
		{"leading dot slash", "./src", "src"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
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
		"foo\\bar",
		"./foo",
		"foo/",
		"foo//bar",
		".\\foo\\\\bar/",
		"././foo",
		"./././bar",
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
		[]byte{0xEF, 0xBB, 0xBF, '*', '.', 'l', 'o', 'g'},
		append([]byte{0xEF, 0xBB, 0xBF}, []byte("*.log\r\nbuild/\r\n")...),
		// Double BOM - this was the fuzz-discovered edge case
		[]byte{0xEF, 0xBB, 0xBF, 0xEF, 0xBB, 0xBF},
		[]byte{0xEF, 0xBB, 0xBF, 0xEF, 0xBB, 0xBF, 0xEF, 0xBB, 0xBF},
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
