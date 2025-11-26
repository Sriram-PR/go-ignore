package ignore

import (
	"testing"
)

func TestMatchSingleSegment_Literal(t *testing.T) {
	tests := []struct {
		pattern string
		input   string
		want    bool
	}{
		{"foo", "foo", true},
		{"foo", "bar", false},
		{"foo", "foobar", false},
		{"foo", "foo ", false},
		{"foo.log", "foo.log", true},
		{"foo.log", "foo.txt", false},
		{"", "", true},
		{"foo", "", false},
		{"", "foo", false},
	}

	for _, tt := range tests {
		seg := segment{value: tt.pattern, wildcard: false}
		got := matchSingleSegment(seg, tt.input, false)
		if got != tt.want {
			t.Errorf("matchSingleSegment(%q, %q) = %v, want %v",
				tt.pattern, tt.input, got, tt.want)
		}
	}
}

func TestMatchSingleSegment_Wildcard(t *testing.T) {
	tests := []struct {
		pattern string
		input   string
		want    bool
	}{
		// Single * patterns
		{"*", "", true},
		{"*", "foo", true},
		{"*", "foo.log", true},

		// Prefix wildcard
		{"*.log", "foo.log", true},
		{"*.log", "bar.log", true},
		{"*.log", "foo.txt", false},
		{"*.log", ".log", true},
		{"*.log", "log", false},

		// Suffix wildcard
		{"foo*", "foo", true},
		{"foo*", "foobar", true},
		{"foo*", "foo.log", true},
		{"foo*", "bar", false},

		// Middle wildcard
		{"foo*bar", "foobar", true},
		{"foo*bar", "fooxbar", true},
		{"foo*bar", "fooxyzbar", true},
		{"foo*bar", "fooXXXbar", true},
		{"foo*bar", "foobaz", false},
		{"foo*bar", "barfoo", false},

		// Multiple wildcards
		{"*foo*", "foo", true},
		{"*foo*", "xfooy", true},
		{"*foo*", "bar", false},
		{"*.test.*", "foo.test.go", true},
		{"*.test.*", "test.go", false},
		{"*.*.*", "a.b.c", true},
		{"*.*.*", "a.b", false},

		// Complex patterns
		{"test_*.go", "test_foo.go", true},
		{"test_*.go", "test_.go", true},
		{"test_*.go", "test_foo.txt", false},
		{"*_test.go", "foo_test.go", true},
		{"*_test.go", "_test.go", true},
		{"*.min.js", "app.min.js", true},
		{"*.min.js", "minjs", false},
	}

	for _, tt := range tests {
		seg := segment{value: tt.pattern, wildcard: true}
		got := matchSingleSegment(seg, tt.input, false)
		if got != tt.want {
			t.Errorf("matchSingleSegment(%q, %q) = %v, want %v",
				tt.pattern, tt.input, got, tt.want)
		}
	}
}

func TestMatchSingleSegment_CaseInsensitive(t *testing.T) {
	tests := []struct {
		pattern         string
		input           string
		caseInsensitive bool
		want            bool
	}{
		// Case sensitive (default)
		{"Foo", "Foo", false, true},
		{"Foo", "foo", false, false},
		{"Foo", "FOO", false, false},
		{"*.LOG", "test.LOG", false, true},
		{"*.LOG", "test.log", false, false},

		// Case insensitive
		{"Foo", "Foo", true, true},
		{"Foo", "foo", true, true},
		{"Foo", "FOO", true, true},
		{"*.LOG", "test.LOG", true, true},
		{"*.LOG", "test.log", true, true},
		{"Build", "build", true, true},
		{"BUILD", "Build", true, true},
	}

	for _, tt := range tests {
		wildcard := containsWildcard(tt.pattern)
		seg := segment{value: tt.pattern, wildcard: wildcard}
		got := matchSingleSegment(seg, tt.input, tt.caseInsensitive)
		if got != tt.want {
			t.Errorf("matchSingleSegment(%q, %q, caseInsensitive=%v) = %v, want %v",
				tt.pattern, tt.input, tt.caseInsensitive, got, tt.want)
		}
	}
}

func containsWildcard(s string) bool {
	return len(s) > 0 && (s[0] == '*' || s[len(s)-1] == '*' || 
		(len(s) > 1 && s != "**" && indexOf(s, '*') >= 0))
}

func indexOf(s string, c byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == c {
			return i
		}
	}
	return -1
}

func TestMatchGlob(t *testing.T) {
	tests := []struct {
		pattern string
		s       string
		want    bool
	}{
		// No wildcards
		{"foo", "foo", true},
		{"foo", "bar", false},

		// Simple star
		{"*", "", true},
		{"*", "anything", true},

		// Prefix
		{"foo*", "foo", true},
		{"foo*", "foobar", true},
		{"foo*", "fo", false},

		// Suffix
		{"*bar", "bar", true},
		{"*bar", "foobar", true},
		{"*bar", "ba", false},

		// Middle
		{"f*o", "fo", true},
		{"f*o", "foo", true},
		{"f*o", "fxxo", true},
		{"f*o", "fx", false},

		// Multiple stars
		{"*a*", "a", true},
		{"*a*", "ba", true},
		{"*a*", "ab", true},
		{"*a*", "bab", true},
		{"*a*", "b", false},

		// Consecutive stars (treated as single *)
		{"**", "anything", true},
		{"***", "anything", true},
		{"f**o", "fo", true},
		{"f**o", "fxxxo", true},

		// Edge cases
		{"", "", true},
		{"", "x", false},
		{"*", "", true},
		{"a*b*c", "abc", true},
		{"a*b*c", "aXbYc", true},
		{"a*b*c", "aXbYcZ", false},
	}

	for _, tt := range tests {
		got := matchGlob(tt.pattern, tt.s)
		if got != tt.want {
			t.Errorf("matchGlob(%q, %q) = %v, want %v",
				tt.pattern, tt.s, got, tt.want)
		}
	}
}

func TestMatchSegments_Simple(t *testing.T) {
	tests := []struct {
		name     string
		pattern  []segment
		path     []string
		want     bool
	}{
		{
			"empty both",
			[]segment{},
			[]string{},
			true,
		},
		{
			"empty pattern",
			[]segment{},
			[]string{"foo"},
			false,
		},
		{
			"empty path",
			[]segment{{value: "foo"}},
			[]string{},
			false,
		},
		{
			"single literal match",
			[]segment{{value: "foo"}},
			[]string{"foo"},
			true,
		},
		{
			"single literal no match",
			[]segment{{value: "foo"}},
			[]string{"bar"},
			false,
		},
		{
			"two literals match",
			[]segment{{value: "foo"}, {value: "bar"}},
			[]string{"foo", "bar"},
			true,
		},
		{
			"two literals partial",
			[]segment{{value: "foo"}, {value: "bar"}},
			[]string{"foo"},
			false,
		},
		{
			"wildcard segment",
			[]segment{{value: "*.log", wildcard: true}},
			[]string{"test.log"},
			true,
		},
		{
			"path longer than pattern",
			[]segment{{value: "foo"}},
			[]string{"foo", "bar"},
			false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := newMatchContext(0)
			got := matchSegments_(tt.pattern, tt.path, ctx, false)
			if got != tt.want {
				t.Errorf("matchSegments_() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMatchSegments_DoubleStar(t *testing.T) {
	tests := []struct {
		name    string
		pattern []segment
		path    []string
		want    bool
	}{
		{
			"** matches empty",
			[]segment{{doubleStar: true}},
			[]string{},
			true,
		},
		{
			"** matches single",
			[]segment{{doubleStar: true}},
			[]string{"foo"},
			true,
		},
		{
			"** matches many",
			[]segment{{doubleStar: true}},
			[]string{"a", "b", "c", "d"},
			true,
		},
		{
			"**/foo matches foo",
			[]segment{{doubleStar: true}, {value: "foo"}},
			[]string{"foo"},
			true,
		},
		{
			"**/foo matches x/foo",
			[]segment{{doubleStar: true}, {value: "foo"}},
			[]string{"x", "foo"},
			true,
		},
		{
			"**/foo matches a/b/c/foo",
			[]segment{{doubleStar: true}, {value: "foo"}},
			[]string{"a", "b", "c", "foo"},
			true,
		},
		{
			"**/foo no match bar",
			[]segment{{doubleStar: true}, {value: "foo"}},
			[]string{"bar"},
			false,
		},
		{
			"foo/** matches foo",
			[]segment{{value: "foo"}, {doubleStar: true}},
			[]string{"foo"},
			true,
		},
		{
			"foo/** matches foo/bar",
			[]segment{{value: "foo"}, {doubleStar: true}},
			[]string{"foo", "bar"},
			true,
		},
		{
			"foo/** matches foo/a/b/c",
			[]segment{{value: "foo"}, {doubleStar: true}},
			[]string{"foo", "a", "b", "c"},
			true,
		},
		{
			"foo/** no match bar",
			[]segment{{value: "foo"}, {doubleStar: true}},
			[]string{"bar"},
			false,
		},
		{
			"a/**/b matches a/b",
			[]segment{{value: "a"}, {doubleStar: true}, {value: "b"}},
			[]string{"a", "b"},
			true,
		},
		{
			"a/**/b matches a/x/b",
			[]segment{{value: "a"}, {doubleStar: true}, {value: "b"}},
			[]string{"a", "x", "b"},
			true,
		},
		{
			"a/**/b matches a/x/y/z/b",
			[]segment{{value: "a"}, {doubleStar: true}, {value: "b"}},
			[]string{"a", "x", "y", "z", "b"},
			true,
		},
		{
			"a/**/b no match a/x",
			[]segment{{value: "a"}, {doubleStar: true}, {value: "b"}},
			[]string{"a", "x"},
			false,
		},
		{
			"a/**/b/**/c matches a/b/c",
			[]segment{
				{value: "a"}, {doubleStar: true},
				{value: "b"}, {doubleStar: true},
				{value: "c"},
			},
			[]string{"a", "b", "c"},
			true,
		},
		{
			"a/**/b/**/c matches a/x/b/y/c",
			[]segment{
				{value: "a"}, {doubleStar: true},
				{value: "b"}, {doubleStar: true},
				{value: "c"},
			},
			[]string{"a", "x", "b", "y", "c"},
			true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := newMatchContext(0)
			got := matchSegments_(tt.pattern, tt.path, ctx, false)
			if got != tt.want {
				t.Errorf("matchSegments_() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMatchRule_Basic(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		path    string
		isDir   bool
		want    bool
	}{
		// Simple patterns
		{"literal match", "foo", "foo", false, true},
		{"literal no match", "foo", "bar", false, false},
		{"literal in subdir", "foo", "src/foo", false, true}, // floating
		{"literal deep", "foo", "a/b/c/foo", false, true},

		// Wildcard patterns
		{"wildcard match", "*.log", "test.log", false, true},
		{"wildcard in subdir", "*.log", "src/test.log", false, true},
		{"wildcard no match", "*.log", "test.txt", false, false},

		// Directory patterns
		{"dir pattern on dir", "build/", "build", true, true},
		{"dir pattern on file", "build/", "build", false, false},
		{"dir pattern nested", "build/", "src/build", true, true},
		// Files INSIDE directories should also be ignored
		{"dir pattern file inside", "build/", "build/output.js", false, true},
		{"dir pattern file deep inside", "build/", "build/a/b/c.js", false, true},
		{"dir pattern nested file inside", "build/", "src/build/output.js", false, true},

		// Anchored patterns (contain /)
		{"anchored match", "src/temp", "src/temp", false, true},
		{"anchored no deep match", "src/temp", "lib/src/temp", false, false},

		// Leading / anchor
		{"leading slash match", "/temp", "temp", false, true},
		{"leading slash no match", "/temp", "src/temp", false, false},

		// Double-star patterns
		{"doublestar prefix", "**/foo", "foo", false, true},
		{"doublestar prefix nested", "**/foo", "a/b/foo", false, true},
		{"doublestar suffix", "foo/**", "foo/bar", false, true},
		{"doublestar suffix deep", "foo/**", "foo/a/b/c", false, true},
		{"doublestar middle", "a/**/b", "a/b", false, true},
		{"doublestar middle nested", "a/**/b", "a/x/y/b", false, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, _ := parseLine(tt.pattern, 1, "")
			if r == nil {
				t.Fatalf("parseLine(%q) returned nil", tt.pattern)
			}
			path := normalizePath(tt.path)
			pathSegs := splitPath(path)
			got := matchRule(r, path, pathSegs, tt.isDir, false, 0)
			if got != tt.want {
				t.Errorf("matchRule(%q, %q, isDir=%v) = %v, want %v",
					tt.pattern, tt.path, tt.isDir, got, tt.want)
			}
		})
	}
}

func TestMatchRule_BasePath(t *testing.T) {
	tests := []struct {
		name     string
		pattern  string
		basePath string
		path     string
		want     bool
	}{
		// Pattern in root .gitignore
		{"root pattern matches root", "*.log", "", "test.log", true},
		{"root pattern matches nested", "*.log", "", "src/test.log", true},

		// Pattern in nested .gitignore
		{"nested pattern matches in scope", "*.log", "src", "src/test.log", true},
		{"nested pattern no match outside", "*.log", "src", "test.log", false},
		{"nested pattern no match sibling", "*.log", "src", "lib/test.log", false},
		{"nested pattern deep match", "*.log", "src", "src/sub/test.log", true},

		// Anchored patterns with basePath
		{"anchored in nested", "foo/bar", "src", "src/foo/bar", true},
		{"anchored in nested no deep", "foo/bar", "src", "src/lib/foo/bar", false},

		// Deep basePath
		{"deep basePath", "*.tmp", "src/lib/internal", "src/lib/internal/test.tmp", true},
		{"deep basePath no match", "*.tmp", "src/lib/internal", "src/lib/test.tmp", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, _ := parseLine(tt.pattern, 1, tt.basePath)
			if r == nil {
				t.Fatalf("parseLine(%q) returned nil", tt.pattern)
			}
			path := normalizePath(tt.path)
			pathSegs := splitPath(path)
			got := matchRule(r, path, pathSegs, false, false, 0)
			if got != tt.want {
				t.Errorf("matchRule(%q, basePath=%q, path=%q) = %v, want %v",
					tt.pattern, tt.basePath, tt.path, got, tt.want)
			}
		})
	}
}

func TestMatchContext_Limit(t *testing.T) {
	// Test that backtrack limit prevents runaway matching
	ctx := newMatchContext(5)

	for i := 0; i < 5; i++ {
		if !ctx.tick() {
			t.Errorf("tick() returned false at iteration %d, expected true", i+1)
		}
	}

	// 6th tick should fail
	if ctx.tick() {
		t.Error("tick() should return false after limit exceeded")
	}
}

func TestMatchContext_Unlimited(t *testing.T) {
	// Test unlimited mode (-1)
	ctx := newMatchContext(-1)

	for i := 0; i < 100000; i++ {
		if !ctx.tick() {
			t.Errorf("tick() returned false at iteration %d with unlimited mode", i)
			break
		}
	}
}

func TestMatchRule_PathologicalPattern(t *testing.T) {
	// Pattern with multiple ** that could cause exponential backtracking
	pattern := "a/**/b/**/c/**/d"
	r, _ := parseLine(pattern, 1, "")
	if r == nil {
		t.Fatal("parseLine returned nil")
	}

	// Create a deep path that doesn't match
	path := "a/x/x/x/x/x/x/x/x/x/x/x/x/x/x/x/b/x/x/x/x/c/x/x/x/e"
	pathSegs := splitPath(path)

	// Should complete without hanging (due to backtrack limit)
	got := matchRule(r, path, pathSegs, false, false, 1000)
	if got {
		t.Error("expected no match for pathological pattern")
	}
}

func TestMatchRule_CaseInsensitive(t *testing.T) {
	tests := []struct {
		pattern         string
		path            string
		caseInsensitive bool
		want            bool
	}{
		{"Build", "build", false, false},
		{"Build", "build", true, true},
		{"BUILD", "Build", true, true},
		{"*.LOG", "test.log", false, false},
		{"*.LOG", "test.log", true, true},
		{"src/BUILD", "src/build", false, false},
		{"src/BUILD", "src/build", true, true},
	}

	for _, tt := range tests {
		t.Run(tt.pattern+"_"+tt.path, func(t *testing.T) {
			r, _ := parseLine(tt.pattern, 1, "")
			if r == nil {
				t.Fatalf("parseLine(%q) returned nil", tt.pattern)
			}
			path := normalizePath(tt.path)
			pathSegs := splitPath(path)
			got := matchRule(r, path, pathSegs, false, tt.caseInsensitive, 0)
			if got != tt.want {
				t.Errorf("matchRule(%q, %q, caseInsensitive=%v) = %v, want %v",
					tt.pattern, tt.path, tt.caseInsensitive, got, tt.want)
			}
		})
	}
}

func TestSplitPath(t *testing.T) {
	tests := []struct {
		path string
		want []string
	}{
		{"", []string{}},
		{"foo", []string{"foo"}},
		{"foo/bar", []string{"foo", "bar"}},
		{"foo/bar/baz", []string{"foo", "bar", "baz"}},
		{"/foo", []string{"foo"}},       // leading slash
		{"foo/", []string{"foo"}},       // trailing slash
		{"foo//bar", []string{"foo", "bar"}}, // double slash
		{"/", []string{}},
	}

	for _, tt := range tests {
		got := splitPath(tt.path)
		if len(got) != len(tt.want) {
			t.Errorf("splitPath(%q) = %v, want %v", tt.path, got, tt.want)
			continue
		}
		for i := range got {
			if got[i] != tt.want[i] {
				t.Errorf("splitPath(%q)[%d] = %q, want %q", tt.path, i, got[i], tt.want[i])
			}
		}
	}
}

// TestMatchRule_DirectoryContents tests that files inside ignored directories are also ignored
func TestMatchRule_DirectoryContents(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		path    string
		isDir   bool
		want    bool
	}{
		// Basic directory patterns
		{"node_modules dir", "node_modules/", "node_modules", true, true},
		{"node_modules file inside", "node_modules/", "node_modules/lodash/index.js", false, true},
		{"node_modules deep file", "node_modules/", "node_modules/a/b/c/d.js", false, true},
		{"node_modules file not inside", "node_modules/", "node_modules.json", false, false},

		// Floating directory patterns
		{"vendor anywhere dir", "vendor/", "vendor", true, true},
		{"vendor anywhere nested dir", "vendor/", "lib/vendor", true, true},
		{"vendor anywhere file inside", "vendor/", "vendor/lib/file.go", false, true},
		{"vendor nested file inside", "vendor/", "lib/vendor/file.go", false, true},

		// Anchored directory patterns
		{"/build dir", "/build/", "build", true, true},
		{"/build file inside", "/build/", "build/output.js", false, true},
		{"/build nested not match", "/build/", "src/build/output.js", false, false},

		// Directory patterns with wildcards
		{"*.d/ dir", "*.d/", "test.d", true, true},
		{"*.d/ file inside", "*.d/", "test.d/file.txt", false, true},

		// Hidden directories
		{".idea dir", ".idea/", ".idea", true, true},
		{".idea file inside", ".idea/", ".idea/workspace.xml", false, true},
		{".vscode nested file", ".vscode/", ".vscode/settings.json", false, true},

		// Nested directory patterns
		{"src/build/ dir", "src/build/", "src/build", true, true},
		{"src/build/ file inside", "src/build/", "src/build/output.js", false, true},
		{"src/build/ deep file", "src/build/", "src/build/a/b/c.js", false, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, _ := parseLine(tt.pattern, 1, "")
			if r == nil {
				t.Fatalf("parseLine(%q) returned nil", tt.pattern)
			}
			path := normalizePath(tt.path)
			pathSegs := splitPath(path)
			got := matchRule(r, path, pathSegs, tt.isDir, false, 0)
			if got != tt.want {
				t.Errorf("matchRule(%q, %q, isDir=%v) = %v, want %v",
					tt.pattern, tt.path, tt.isDir, got, tt.want)
			}
		})
	}
}

// Spec examples from Section 6
func TestMatchRule_SpecExamples(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		path    string
		isDir   bool
		want    bool
	}{
		// Plain name - match anywhere
		{"plain name root", "debug.log", "debug.log", false, true},
		{"plain name nested", "debug.log", "src/debug.log", false, true},

		// Leading / - match only at base
		{"leading slash match", "/debug.log", "debug.log", false, true},
		{"leading slash no match nested", "/debug.log", "src/debug.log", false, false},

		// Trailing / - directories only
		{"trailing slash dir", "build/", "build", true, true},
		{"trailing slash file", "build/", "build", false, false},

		// * - any chars within segment
		{"star prefix", "*.log", "foo.log", false, true},
		{"star prefix 2", "*.log", "bar.log", false, true},

		// ** - zero or more directories
		{"doublestar logs", "**/logs", "logs", false, true},
		{"doublestar logs nested", "**/logs", "src/logs", false, true},
		{"doublestar logs deep", "**/logs", "a/b/logs", false, true},

		// **/ prefix - any directory
		{"doublestar prefix temp", "**/temp", "temp", false, true},
		{"doublestar prefix temp nested", "**/temp", "x/temp", false, true},
		{"doublestar prefix temp deep", "**/temp", "x/y/temp", false, true},

		// /** suffix - everything inside
		{"doublestar suffix", "build/**", "build/a", false, true},
		{"doublestar suffix deep", "build/**", "build/a/b/c", false, true},

		// /**/ middle - any depth between
		{"doublestar middle direct", "a/**/b", "a/b", false, true},
		{"doublestar middle one", "a/**/b", "a/x/b", false, true},
		{"doublestar middle deep", "a/**/b", "a/x/y/b", false, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, _ := parseLine(tt.pattern, 1, "")
			if r == nil {
				t.Fatalf("parseLine(%q) returned nil", tt.pattern)
			}
			path := normalizePath(tt.path)
			pathSegs := splitPath(path)
			got := matchRule(r, path, pathSegs, tt.isDir, false, 0)
			if got != tt.want {
				t.Errorf("pattern %q, path %q: got %v, want %v",
					tt.pattern, tt.path, got, tt.want)
			}
		})
	}
}

func BenchmarkMatchGlob_Simple(b *testing.B) {
	for i := 0; i < b.N; i++ {
		matchGlob("*.log", "test.log")
	}
}

func BenchmarkMatchGlob_Complex(b *testing.B) {
	for i := 0; i < b.N; i++ {
		matchGlob("*test*spec*.go", "my_test_spec_file.go")
	}
}

func BenchmarkMatchSegments_Simple(b *testing.B) {
	pattern := []segment{{value: "src"}, {value: "*.go", wildcard: true}}
	path := []string{"src", "main.go"}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ctx := newMatchContext(0)
		matchSegments_(pattern, path, ctx, false)
	}
}

func BenchmarkMatchSegments_DoubleStar(b *testing.B) {
	pattern := []segment{{doubleStar: true}, {value: "test"}, {doubleStar: true}, {value: "*.go", wildcard: true}}
	path := []string{"src", "lib", "test", "unit", "foo_test.go"}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ctx := newMatchContext(0)
		matchSegments_(pattern, path, ctx, false)
	}
}