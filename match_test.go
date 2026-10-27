package ignore

import (
	"fmt"
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
		t.Run(tt.pattern+"_"+tt.input, func(t *testing.T) {
			seg := segment{value: tt.pattern, wildcard: false}
			got := matchSingleSegment(seg, tt.input)
			if got != tt.want {
				t.Errorf("matchSingleSegment(%q, %q) = %v, want %v",
					tt.pattern, tt.input, got, tt.want)
			}
		})
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
		t.Run(tt.pattern+"_"+tt.input, func(t *testing.T) {
			seg := segment{value: tt.pattern, wildcard: true}
			got := matchSingleSegment(seg, tt.input)
			if got != tt.want {
				t.Errorf("matchSingleSegment(%q, %q) = %v, want %v",
					tt.pattern, tt.input, got, tt.want)
			}
		})
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
		name := fmt.Sprintf("%s_%s_ci=%v", tt.pattern, tt.input, tt.caseInsensitive)
		t.Run(name, func(t *testing.T) {
			m := NewWithOptions(MatcherOptions{CaseInsensitive: tt.caseInsensitive})
			m.AddPatterns("", []byte(tt.pattern+"\n"))
			if got := m.Match(tt.input, false); got != tt.want {
				t.Errorf("pattern %q, Match(%q) with CaseInsensitive=%v = %v, want %v",
					tt.pattern, tt.input, tt.caseInsensitive, got, tt.want)
			}
		})
	}
}

func TestMatchSegments_Simple(t *testing.T) {
	tests := []struct {
		name    string
		pattern []segment
		path    []string
		want    bool
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
			got := matchSegments(tt.pattern, tt.path)
			if got != tt.want {
				t.Errorf("matchSegments() = %v, want %v", got, tt.want)
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
			false, // trailing ** requires at least one segment
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
			"foo/** does not match foo itself",
			[]segment{{value: "foo"}, {doubleStar: true}},
			[]string{"foo"},
			false, // trailing ** requires content inside, not the dir itself
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
			got := matchSegments(tt.pattern, tt.path)
			if got != tt.want {
				t.Errorf("matchSegments() = %v, want %v", got, tt.want)
			}
		})
	}
}

// Many ** segments need neither recursion nor a depth limit.
func TestMatchSegments_ManyDoubleStars(t *testing.T) {
	pattern := make([]segment, 251)
	for i := range 250 {
		pattern[i] = segment{doubleStar: true}
	}
	pattern[250] = segment{value: "target"}
	path := make([]string, 250)
	for i := range 249 {
		path[i] = "a"
	}
	path[249] = "target"
	if !matchSegments(pattern, path) {
		t.Error("250 ** then target: want a match")
	}
	if matchSegments(pattern, path[:249]) {
		t.Error("path without target: want no match")
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
		// A dir-only rule names the directory, not the files inside it;
		// Match covers those (TestMatchRule_DirectoryContents).
		{"dir pattern file inside", "build/", "build/output.js", false, false},
		{"dir pattern file deep inside", "build/", "build/a/b/c.js", false, false},
		{"dir pattern nested file inside", "build/", "src/build/output.js", false, false},

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
			r, _ := parseLine(tt.pattern, 1, "", "")
			if r == nil {
				t.Fatalf("parseLine(%q) returned nil", tt.pattern)
			}
			path := normalizePath(tt.path)
			pathSegs := splitPath(path)
			got := matchRule(r, path, pathSegs, tt.isDir)
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
			rules, _, _ := parseLinesAll(tt.basePath, []byte(tt.pattern), -1, -1, "")
			if len(rules) != 1 {
				t.Fatalf("parseLinesAll(%q) = %d rules, want 1", tt.pattern, len(rules))
			}
			path := normalizePath(tt.path)
			pathSegs := splitPath(path)
			got := matchRule(&rules[0], path, pathSegs, false)
			if got != tt.want {
				t.Errorf("matchRule(%q, basePath=%q, path=%q) = %v, want %v",
					tt.pattern, tt.basePath, tt.path, got, tt.want)
			}
		})
	}
}

func TestMatchRule_PathologicalPattern(t *testing.T) {
	// Pattern with multiple ** that could cause exponential backtracking
	pattern := "a/**/b/**/c/**/d"
	r, _ := parseLine(pattern, 1, "", "")
	if r == nil {
		t.Fatal("parseLine returned nil")
	}

	// Create a deep path that doesn't match
	path := "a/x/x/x/x/x/x/x/x/x/x/x/x/x/x/x/b/x/x/x/x/c/x/x/x/e"
	pathSegs := splitPath(path)

	if matchRule(r, path, pathSegs, false) {
		t.Error("path without d: want no match")
	}
	path += "/d"
	if !matchRule(r, path, splitPath(path), false) {
		t.Error("same path ending in d: want a match")
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
			m := NewWithOptions(MatcherOptions{CaseInsensitive: tt.caseInsensitive})
			m.AddPatterns("", []byte(tt.pattern+"\n"))
			if got := m.Match(tt.path, false); got != tt.want {
				t.Errorf("pattern %q, Match(%q) with CaseInsensitive=%v = %v, want %v",
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
		{"/foo", []string{"foo"}},            // leading slash
		{"foo/", []string{"foo"}},            // trailing slash
		{"foo//bar", []string{"foo", "bar"}}, // double slash
		{"/", []string{}},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("%q", tt.path), func(t *testing.T) {
			got := splitPath(tt.path)
			if len(got) != len(tt.want) {
				t.Errorf("splitPath(%q) = %v, want %v", tt.path, got, tt.want)
				return
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("splitPath(%q)[%d] = %q, want %q", tt.path, i, got[i], tt.want[i])
				}
			}
		})
	}
}

// TestMatchRule_DirectoryContents checks that a dir-only rule names the
// directory, not the files inside it, and that Match still ignores those files
// through its parent-directory check.
func TestMatchRule_DirectoryContents(t *testing.T) {
	tests := []struct {
		name      string
		pattern   string
		path      string
		isDir     bool
		wantRule  bool
		wantMatch bool
	}{
		// Basic directory patterns
		{"node_modules dir", "node_modules/", "node_modules", true, true, true},
		{"node_modules file inside", "node_modules/", "node_modules/lodash/index.js", false, false, true},
		{"node_modules deep file", "node_modules/", "node_modules/a/b/c/d.js", false, false, true},
		{"node_modules file not inside", "node_modules/", "node_modules.json", false, false, false},

		// Floating directory patterns
		{"vendor anywhere dir", "vendor/", "vendor", true, true, true},
		{"vendor anywhere nested dir", "vendor/", "lib/vendor", true, true, true},
		{"vendor anywhere file inside", "vendor/", "vendor/lib/file.go", false, false, true},
		{"vendor nested file inside", "vendor/", "lib/vendor/file.go", false, false, true},

		{"build file inside", "build/", "build/output.js", false, false, true},
		{"build deep file inside", "build/", "build/a/b/c.js", false, false, true},
		{"build nested file inside", "build/", "src/build/output.js", false, false, true},

		// Anchored directory patterns
		{"/build dir", "/build/", "build", true, true, true},
		{"/build file inside", "/build/", "build/output.js", false, false, true},
		{"/build nested not match", "/build/", "src/build/output.js", false, false, false},

		// Directory patterns with wildcards
		{"*.d/ dir", "*.d/", "test.d", true, true, true},
		{"*.d/ file inside", "*.d/", "test.d/file.txt", false, false, true},

		// Hidden directories
		{".idea dir", ".idea/", ".idea", true, true, true},
		{".idea file inside", ".idea/", ".idea/workspace.xml", false, false, true},
		{".vscode nested file", ".vscode/", ".vscode/settings.json", false, false, true},

		// Nested directory patterns
		{"src/build/ dir", "src/build/", "src/build", true, true, true},
		{"src/build/ file inside", "src/build/", "src/build/output.js", false, false, true},
		{"src/build/ deep file", "src/build/", "src/build/a/b/c.js", false, false, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, _ := parseLine(tt.pattern, 1, "", "")
			if r == nil {
				t.Fatalf("parseLine(%q) returned nil", tt.pattern)
			}
			path := normalizePath(tt.path)
			pathSegs := splitPath(path)
			if got := matchRule(r, path, pathSegs, tt.isDir); got != tt.wantRule {
				t.Errorf("matchRule(%q, %q, isDir=%v) = %v, want %v",
					tt.pattern, tt.path, tt.isDir, got, tt.wantRule)
			}
			m := New()
			m.AddPatterns("", []byte(tt.pattern+"\n"))
			if got := m.Match(tt.path, tt.isDir); got != tt.wantMatch {
				t.Errorf("Match(%q, %v) with %q = %v, want %v",
					tt.path, tt.isDir, tt.pattern, got, tt.wantMatch)
			}
		})
	}
}

// TestMatchRule_EscapedWildcards tests that escaped wildcards match literally
func TestMatchRule_EscapedWildcards(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		path    string
		want    bool
	}{
		// \* matches literal asterisk in filename
		{"escaped star matches", "\\*.txt", "*.txt", true},
		{"escaped star no wildcard", "\\*.txt", "foo.txt", false},
		// \? matches literal question mark
		{"escaped question matches", "\\?.txt", "?.txt", true},
		{"escaped question no single", "\\?.txt", "a.txt", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, w := parseLine(tt.pattern, 1, "", "")
			if w != nil {
				t.Fatalf("parseLine(%q) warning: %v", tt.pattern, w)
			}
			if r == nil {
				t.Fatalf("parseLine(%q) returned nil", tt.pattern)
			}
			path := normalizePath(tt.path)
			pathSegs := splitPath(path)
			got := matchRule(r, path, pathSegs, false)
			if got != tt.want {
				t.Errorf("matchRule(%q, %q) = %v, want %v",
					tt.pattern, tt.path, got, tt.want)
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

		// /** suffix - everything inside (not the directory itself)
		{"doublestar suffix", "build/**", "build/a", false, true},
		{"doublestar suffix deep", "build/**", "build/a/b/c", false, true},
		{"doublestar suffix dir itself", "build/**", "build", true, false},

		// /**/ middle - any depth between
		{"doublestar middle direct", "a/**/b", "a/b", false, true},
		{"doublestar middle one", "a/**/b", "a/x/b", false, true},
		{"doublestar middle deep", "a/**/b", "a/x/y/b", false, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, _ := parseLine(tt.pattern, 1, "", "")
			if r == nil {
				t.Fatalf("parseLine(%q) returned nil", tt.pattern)
			}
			path := normalizePath(tt.path)
			pathSegs := splitPath(path)
			got := matchRule(r, path, pathSegs, tt.isDir)
			if got != tt.want {
				t.Errorf("pattern %q, path %q: got %v, want %v",
					tt.pattern, tt.path, got, tt.want)
			}
		})
	}
}

// In git, ** followed by \/ skips the zero-directory shortcut
// (wildmatch.c), so it must match at least one directory.
func TestMatchSegments_MinOne(t *testing.T) {
	tests := []struct {
		pattern, path string
		want          bool
	}{
		{"**\\/b", "b", false},
		{"**\\/b", "a/b", true},
		{"**\\/b", "a/x/b", true},
		{"a/**\\/b", "a/b", false},
		{"a/**\\/b", "a/x/b", true},
		{"**\\/**", "x", false},
		{"**\\/**", "x/y", true},
		{"**/**", "x", true},
		{"**\\/**\\/b", "x/b", false},
		{"**\\/**\\/b", "x/y/b", true},
	}
	for _, tt := range tests {
		segs, bad := parseSegments(tt.pattern)
		if bad != "" {
			t.Fatalf("parseSegments(%q) bad = %q", tt.pattern, bad)
		}
		if got := matchSegments(segs, splitPath(tt.path)); got != tt.want {
			t.Errorf("matchSegments(%q, %q) = %v, want %v", tt.pattern, tt.path, got, tt.want)
		}
	}
}

func TestMatchSegments_AllStars(t *testing.T) {
	for _, pattern := range []string{"**", "***"} {
		segs, bad := parseSegments(pattern)
		if bad != "" {
			t.Fatalf("parseSegments(%q) bad = %q", pattern, bad)
		}
		if !matchSegments(segs, []string{"anything"}) {
			t.Errorf("matchSegments(%q, anything) = false, want true", pattern)
		}
	}
}
