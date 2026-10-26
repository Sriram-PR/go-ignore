package ignore

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestMatch_Basic(t *testing.T) {
	m := New()
	m.AddPatterns("", []byte("*.log\nbuild/\n!important.log\n"))

	tests := []struct {
		name  string
		path  string
		isDir bool
		want  bool
	}{
		{"test.log file", "test.log", false, true},
		{"debug.log file", "debug.log", false, true},
		{"important.log negated", "important.log", false, false}, // negated
		{"main.go no match", "main.go", false, false},
		{"build dir", "build", true, true},
		{"build not a dir", "build", false, false}, // not a dir
		{"src/test.log nested", "src/test.log", false, true},
		{"src/build nested dir", "src/build", true, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := m.Match(tt.path, tt.isDir)
			if got != tt.want {
				t.Errorf("Match(%q, %v) = %v, want %v", tt.path, tt.isDir, got, tt.want)
			}
		})
	}
}

func TestMatch_WindowsPaths(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("backslash-to-slash conversion only applies on Windows")
	}

	m := New()
	m.AddPatterns("", []byte("*.log\nsrc/build/\n"))

	tests := []struct {
		name  string
		path  string
		isDir bool
		want  bool
	}{
		{"src\\test.log file", "src\\test.log", false, true},
		{"src\\build dir", "src\\build", true, true},
		{"src\\lib\\debug.log nested", "src\\lib\\debug.log", false, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := m.Match(tt.path, tt.isDir)
			if got != tt.want {
				t.Errorf("Match(%q, %v) = %v, want %v", tt.path, tt.isDir, got, tt.want)
			}
		})
	}
}

func TestMatch_EmptyPath(t *testing.T) {
	m := New()
	m.AddPatterns("", []byte("*.log\n"))

	if m.Match("", false) {
		t.Error("Match('') should return false")
	}
}

// A nested .gitignore in a mixed-case directory must still apply under
// CaseInsensitive: the path is lowercased before matching, so the rule's
// base path has to be compared case-insensitively too.
func TestMatch_CaseInsensitive_MixedCaseBasePath(t *testing.T) {
	m := NewWithOptions(MatcherOptions{CaseInsensitive: true})
	m.AddPatterns("", []byte("*.LOG\n"))
	m.AddPatterns("Sub", []byte("!x.log\n"))

	tests := []struct {
		path string
		want bool
	}{
		{"Sub/x.log", false},
		{"SUB/X.LOG", false},
		{"Sub/y.log", true},
		{"other/x.log", true},
	}
	for _, tt := range tests {
		if got := m.Match(tt.path, false); got != tt.want {
			t.Errorf("Match(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}

	if r := m.MatchWithReason("Sub/x.log", false); r.BasePath != "Sub" {
		t.Errorf("BasePath = %q, want %q (as written by the caller)", r.BasePath, "Sub")
	}
}

func TestMatch_CaseInsensitive(t *testing.T) {
	m := NewWithOptions(MatcherOptions{CaseInsensitive: true})
	m.AddPatterns("", []byte("BUILD/\n*.LOG\n"))

	tests := []struct {
		name  string
		path  string
		isDir bool
		want  bool
	}{
		{"BUILD uppercase dir", "BUILD", true, true},
		{"build lowercase dir", "build", true, true},
		{"Build mixed dir", "Build", true, true},
		{"test.LOG uppercase ext", "test.LOG", false, true},
		{"test.log lowercase ext", "test.log", false, true},
		{"test.Log mixed ext", "test.Log", false, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := m.Match(tt.path, tt.isDir)
			if got != tt.want {
				t.Errorf("Match(%q, %v) = %v, want %v", tt.path, tt.isDir, got, tt.want)
			}
		})
	}
}

func TestMatch_NestedGitignore(t *testing.T) {
	m := New()

	// Root .gitignore
	m.AddPatterns("", []byte("*.log\n"))

	// src/.gitignore
	m.AddPatterns("src", []byte("*.tmp\n!keep.tmp\n"))

	// src/lib/.gitignore
	m.AddPatterns("src/lib", []byte("*.bak\n"))

	tests := []struct {
		name string
		path string
		want bool
	}{
		// Root patterns apply everywhere
		{"root test.log", "test.log", true},
		{"nested src/test.log", "src/test.log", true},
		{"deeply nested src/lib/test.log", "src/lib/test.log", true},

		// src patterns only in src/
		{"test.tmp not in src", "test.tmp", false},               // not in src/
		{"src/test.tmp in src", "src/test.tmp", true},            // in src/
		{"src/keep.tmp negated", "src/keep.tmp", false},          // negated in src/
		{"src/lib/test.tmp inherited", "src/lib/test.tmp", true}, // inherited

		// src/lib patterns only in src/lib/
		{"test.bak not in src/lib", "test.bak", false},            // not in src/lib/
		{"src/test.bak not in src/lib", "src/test.bak", false},    // not in src/lib/
		{"src/lib/test.bak in src/lib", "src/lib/test.bak", true}, // in src/lib/
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := m.Match(tt.path, false)
			if got != tt.want {
				t.Errorf("Match(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestMatch_DotDotDoesNotBypassBasePath(t *testing.T) {
	m := New()
	m.AddPatterns("src", []byte("secret.txt\n"))

	// src/../secret.txt resolves to secret.txt which is NOT under src/
	if m.Match("src/../secret.txt", false) {
		t.Error("src/../secret.txt should not match pattern scoped to src/")
	}
	// Direct secret.txt at root should not match src-scoped pattern
	if m.Match("secret.txt", false) {
		t.Error("secret.txt at root should not match pattern scoped to src/")
	}
	// src/secret.txt should still match
	if !m.Match("src/secret.txt", false) {
		t.Error("src/secret.txt should match pattern scoped to src/")
	}
}

func TestMatchWithReason_Basic(t *testing.T) {
	m := New()
	m.AddPatterns("", []byte("*.log\n!important.log\nbuild/\n"))

	tests := []struct {
		name    string
		path    string
		isDir   bool
		matched bool
		ignored bool
		rule    string
		line    int
		negated bool
	}{
		{"regular ignore", "debug.log", false, true, true, "*.log", 1, false},
		{"negation", "important.log", false, true, false, "!important.log", 2, true},
		{"no match", "main.go", false, false, false, "", 0, false},
		{"directory", "build", true, true, true, "build/", 3, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := m.MatchWithReason(tt.path, tt.isDir)
			if result.Matched != tt.matched {
				t.Errorf("Matched = %v, want %v", result.Matched, tt.matched)
			}
			if result.Ignored != tt.ignored {
				t.Errorf("Ignored = %v, want %v", result.Ignored, tt.ignored)
			}
			if result.Rule != tt.rule {
				t.Errorf("Rule = %q, want %q", result.Rule, tt.rule)
			}
			if result.Line != tt.line {
				t.Errorf("Line = %d, want %d", result.Line, tt.line)
			}
			if result.Negated() != tt.negated {
				t.Errorf("Negated() = %v, want %v", result.Negated(), tt.negated)
			}
			// Negated() must agree with its derivation across the
			// no-match / ignored / negated states.
			if result.Negated() && (!result.Matched || result.Ignored) {
				t.Errorf("Negated() inconsistent with Matched/Ignored: matched=%v ignored=%v",
					result.Matched, result.Ignored)
			}
		})
	}
}

func TestMatchWithReason_BasePath(t *testing.T) {
	m := New()
	m.AddPatterns("", []byte("*.log\n"))
	m.AddPatterns("src", []byte("*.tmp\n"))

	// Root pattern
	result := m.MatchWithReason("test.log", false)
	if result.BasePath != "" {
		t.Errorf("BasePath = %q, want empty", result.BasePath)
	}

	// Nested pattern
	result = m.MatchWithReason("src/test.tmp", false)
	if result.BasePath != "src" {
		t.Errorf("BasePath = %q, want %q", result.BasePath, "src")
	}
}

func TestMatchWithReason_LastMatchWins(t *testing.T) {
	m := New()
	m.AddPatterns("", []byte("*.log\n*.log\n!test.log\ntest.log\n"))

	// Last matching rule should win
	result := m.MatchWithReason("test.log", false)
	if result.Rule != "test.log" {
		t.Errorf("Rule = %q, want %q (last match)", result.Rule, "test.log")
	}
	if result.Line != 4 {
		t.Errorf("Line = %d, want 4", result.Line)
	}
	if !result.Ignored {
		t.Error("Should be ignored (last rule is not negation)")
	}
}

func TestMatcher_RealWorld(t *testing.T) {
	m := New()

	// Simulate typical .gitignore
	m.AddPatterns("", []byte(`
# Dependencies
node_modules/
vendor/

# Build outputs
build/
dist/
*.exe

# Logs
*.log
logs/

# IDE
.idea/
.vscode/
*.swp

# OS
.DS_Store
Thumbs.db

# Environment
.env
.env.local
.env.*.local

# Keep
!.gitkeep
`))

	tests := []struct {
		name  string
		path  string
		isDir bool
		want  bool
	}{
		{"node_modules dir", "node_modules", true, true},
		{"node_modules/lodash/index.js", "node_modules/lodash/index.js", false, true},
		{"vendor/lib/file.go", "vendor/lib/file.go", false, true},
		{"build/app.exe", "build/app.exe", false, true},
		{"dist/bundle.js", "dist/bundle.js", false, true},
		{"app.exe", "app.exe", false, true},
		{"debug.log", "debug.log", false, true},
		{"logs/error.log", "logs/error.log", false, true},
		{".idea/workspace.xml", ".idea/workspace.xml", false, true},
		{".vscode/settings.json", ".vscode/settings.json", false, true},
		{"file.swp", "file.swp", false, true},
		{".DS_Store", ".DS_Store", false, true},
		{"Thumbs.db", "Thumbs.db", false, true},
		{".env", ".env", false, true},
		{".env.local", ".env.local", false, true},
		{".env.production.local", ".env.production.local", false, true},
		{".gitkeep negated", ".gitkeep", false, false}, // negated
		{"src/main.go no match", "src/main.go", false, false},
		{"README.md no match", "README.md", false, false},
		{"package.json no match", "package.json", false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := m.Match(tt.path, tt.isDir)
			if got != tt.want {
				t.Errorf("Match(%q, %v) = %v, want %v", tt.path, tt.isDir, got, tt.want)
			}
		})
	}
}

func TestMatcher_GitDirectory(t *testing.T) {
	m := New()

	// User explicitly adds .git/
	m.AddPatterns("", []byte(".git/\n"))

	tests := []struct {
		name  string
		path  string
		isDir bool
		want  bool
	}{
		{".git dir", ".git", true, true},
		{".git/config", ".git/config", false, true},
		{".git/objects/pack/pack-123.idx", ".git/objects/pack/pack-123.idx", false, true},
		{".gitignore not .git", ".gitignore", false, false}, // not .git/
		{".github/workflows/ci.yml", ".github/workflows/ci.yml", false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := m.Match(tt.path, tt.isDir)
			if got != tt.want {
				t.Errorf("Match(%q, %v) = %v, want %v", tt.path, tt.isDir, got, tt.want)
			}
		})
	}
}

func TestMatcher_DoubleStarPatterns(t *testing.T) {
	m := New()
	m.AddPatterns("", []byte(`
**/logs
**/logs/**
**/.cache
a/**/b
foo/**
`))

	tests := []struct {
		name string
		path string
		want bool
	}{
		{"logs root", "logs", true},
		{"src/logs nested", "src/logs", true},
		{"a/b/c/logs deeply nested", "a/b/c/logs", true},
		{"logs/debug.log under logs", "logs/debug.log", true},
		{"src/logs/error.log nested under logs", "src/logs/error.log", true},
		{".cache root", ".cache", true},
		{"home/.cache nested", "home/.cache", true},
		{"a/b middle star", "a/b", true},
		{"a/x/b one segment star", "a/x/b", true},
		{"a/x/y/z/b many segments star", "a/x/y/z/b", true},
		{"foo/bar under foo", "foo/bar", true},
		{"foo/bar/baz deeply under foo", "foo/bar/baz", true},
		// foo/** should NOT match foo directory itself
		{"foo dir itself not matched by foo/**", "foo", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := m.Match(tt.path, false)
			if got != tt.want {
				t.Errorf("Match(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

// Examples from spec Section 4
func TestMatchWithReason_SpecExamples(t *testing.T) {
	m := New()
	m.AddPatterns("", []byte(`
*.log
!important.log
build/
`))

	// Example 1: Regular ignore
	result := m.MatchWithReason("debug.log", false)
	if !result.Matched || !result.Ignored || result.Rule != "*.log" || result.Negated() {
		t.Errorf("Example 1 failed: %+v", result)
	}

	// Example 2: Negation re-includes file
	result = m.MatchWithReason("important.log", false)
	if !result.Matched || result.Ignored || result.Rule != "!important.log" || !result.Negated() {
		t.Errorf("Example 2 failed: %+v", result)
	}

	// Example 3: No match
	result = m.MatchWithReason("main.go", false)
	if result.Matched || result.Ignored || result.Rule != "" {
		t.Errorf("Example 3 failed: %+v", result)
	}

	// Example 4: Directory match
	result = m.MatchWithReason("build", true)
	if !result.Matched || !result.Ignored || result.Rule != "build/" {
		t.Errorf("Example 4 failed: %+v", result)
	}
}

// With checkParents off, match skips the parent-directory check, so a
// negation inside an ignored directory stands. WalkDir relies on this: it
// never visits a path under an ignored directory.
func TestMatch_CheckParentsFlag(t *testing.T) {
	m := New()
	m.AddPatterns("", []byte("build/\n!build/keep.txt\n"))
	if got := m.match("build/keep.txt", false, true); !got.Ignored || got.Rule != "build/" {
		t.Errorf("match(checkParents=true) = %+v, want ignored by build/", got)
	}
	if got := m.match("build/keep.txt", false, false); got.Ignored || got.Rule != "!build/keep.txt" {
		t.Errorf("match(checkParents=false) = %+v, want re-included by !build/keep.txt", got)
	}
}

// A path under an ignored directory is ignored with or without a negation,
// and the parent's rule is reported, as git check-ignore -v does.
func TestMatch_IgnoredParentCoversContents(t *testing.T) {
	tests := []struct {
		patterns string
		path     string
		isDir    bool
		wantRule string
	}{
		{"build\n", "build/app.js", false, "build"},
		{"build\n", "src/build/a/b.js", false, "build"},
		{"/out\n", "out/x/y", true, "/out"},
		{"build/\n*.js\n", "build/app.js", false, "build/"},
		{"*.d\n!a.d/keep\n", "a.d/keep", false, "*.d"},
	}
	for _, tt := range tests {
		m := New()
		m.AddPatterns("", []byte(tt.patterns))
		got := m.MatchWithReason(tt.path, tt.isDir)
		if !got.Ignored || got.Rule != tt.wantRule {
			t.Errorf("patterns %q: MatchWithReason(%q, %v) = %+v, want ignored by %q",
				tt.patterns, tt.path, tt.isDir, got, tt.wantRule)
		}
	}
}

// A dir-only negation re-includes the directory, not the files inside it, as
// in git. With the allowlist idiom only the re-included files are kept.
func TestMatch_DirOnlyNegationReincludesDirOnly(t *testing.T) {
	tests := []struct {
		patterns string
		path     string
		isDir    bool
		want     bool
	}{
		{"*\n!*/\n!*.go\n", "a", true, false},
		{"*\n!*/\n!*.go\n", "a/x", false, true},
		{"*\n!*/\n!*.go\n", "a/y.go", false, false},
		{"*\n!*/\n!*.go\n", "a/b/x", false, true},
		{"*.txt\n!sub/\n", "sub", true, false},
		{"*.txt\n!sub/\n", "sub/a.txt", false, true},
	}
	for _, tt := range tests {
		m := New()
		m.AddPatterns("", []byte(tt.patterns))
		if got := m.Match(tt.path, tt.isDir); got != tt.want {
			t.Errorf("patterns %q: Match(%q, %v) = %v, want %v", tt.patterns, tt.path, tt.isDir, got, tt.want)
		}
	}
}

// A parent's verdict uses every rule: *, re-included by !*/ for directories,
// leaves no parent ignored, so !*.go decides for the file.
func TestMatch_ParentCheckWithChainRule(t *testing.T) {
	m := New()
	m.AddPatterns("", []byte("*\n**/q/**/q/**/q/**/z\n!*/\n!*.go\n"))
	if got := m.MatchWithReason("a/b/c/d/e/f/x.go", false); got.Ignored || got.Rule != "!*.go" {
		t.Errorf("MatchWithReason = %+v, want re-included by !*.go", got)
	}
}

// A . segment names the directory it is in, as in git: a/. is a, and . is
// the root, which matches nothing. So a rule "." never matches a path.
func TestMatch_DotSegments(t *testing.T) {
	m := New()
	m.AddPatterns("", []byte(".\nb\n"))
	for path, want := range map[string]bool{".": false, "./.": false, "c/.": false, "b/.": true, "b/./": true, "x/b/.": true} {
		if got := m.Match(path, true); got != want {
			t.Errorf("Match(%q, true) = %v, want %v", path, got, want)
		}
	}
}

func TestASCIILower(t *testing.T) {
	for in, want := range map[string]string{
		"":          "",
		"abc":       "abc",
		"ABC":       "abc",
		"aBc/É\xff": "abc/É\xff",
		"Ǆ":         "Ǆ",
	} {
		if got := asciiLower(in); got != want {
			t.Errorf("asciiLower(%q) = %q, want %q", in, got, want)
		}
	}
	if n := testing.AllocsPerRun(100, func() { _ = asciiLower("already/lower/é") }); n != 0 {
		t.Errorf("asciiLower allocated %v times with nothing to fold", n)
	}
}

// git folds ASCII letters only (sane-ctype.h), and compares other bytes,
// valid UTF-8 or not, as they are.
func TestCaseInsensitive_ASCIIOnly(t *testing.T) {
	tests := []struct {
		pattern, path string
		want          bool
	}{
		{"FOO", "foo", true},
		{"foo", "FOO", true},
		{"É", "é", false},
		{"é", "É", false},
		{"K", "K", false}, // Kelvin sign
		{"K", "k", false},
		{"ǅ", "ǆ", false},
		{"a\xff", "a\xfe", false},
		{"a\xff", "A\xff", true},
		{"a?", "A\xff", true},
	}
	for _, tt := range tests {
		m := NewWithOptions(MatcherOptions{CaseInsensitive: true})
		m.AddPatterns("", []byte(tt.pattern+"\n"))
		if got := m.Match(tt.path, false); got != tt.want {
			t.Errorf("pattern %q, Match(%q) = %v, want %v", tt.pattern, tt.path, got, tt.want)
		}
	}
}

func TestCaseInsensitive_ASCIIOnlyBasePath(t *testing.T) {
	m := NewWithOptions(MatcherOptions{CaseInsensitive: true})
	m.AddPatterns("Dir", []byte("x\n"))
	m.AddPatterns("É", []byte("y\n"))
	if !m.Match("dir/x", false) {
		t.Error("Match(dir/x) = false, want true")
	}
	if !m.Match("É/y", false) {
		t.Error("Match(É/y) = false, want true")
	}
	if m.Match("é/y", false) {
		t.Error("Match(é/y) = true, want false")
	}
}

func TestParentIgnored(t *testing.T) {
	rules, _, _ := parseLines("", []byte("build/\n!build/keep/\nlogs\n"), -1, -1, "")
	tests := []struct {
		path     string
		wantOK   bool
		wantRule string
	}{
		{"build/x/y.o", true, "build/"},
		{"build/keep/a", true, "build/"}, // outermost ignored parent decides
		{"src/logs/a", true, "logs"},
		{"src/a", false, ""},
		{"build", false, ""}, // the path itself is not a parent
	}
	for _, tt := range tests {
		got, _, ok := parentIgnored(rules, nil, tt.path, splitPath(tt.path))
		if ok != tt.wantOK || got.Rule != tt.wantRule {
			t.Errorf("parentIgnored(%q) = %q, %v; want %q, %v", tt.path, got.Rule, ok, tt.wantRule, tt.wantOK)
		}
	}
}

// TestFixtureMatchCorrectness loads fixture files and verifies match behavior.
func TestFixtureMatchCorrectness(t *testing.T) {
	type matchCase struct {
		path  string
		isDir bool
		want  bool
	}

	fixtures := []struct {
		path  string
		cases []matchCase
	}{
		{
			"testdata/simple.gitignore",
			[]matchCase{
				{"test.log", false, true},        // *.log
				{"build", true, true},            // build/
				{"build/output.js", false, true}, // inside build/
				{".DS_Store", false, true},       // .DS_Store
				{"src/main.go", false, false},    // no match
				{"node_modules", true, true},     // node_modules/
				{"Thumbs.db", false, true},       // Thumbs.db
			},
		},
		{
			"testdata/complex.gitignore",
			[]matchCase{
				{"important.log", false, false},        // negated by !important.log
				{"keep.log", false, false},             // negated by !keep.log
				{"debug.log", false, true},             // *.log
				{"src/generated/file.go", false, true}, // src/generated/
				{"docs/api/index.html", false, true},   // docs/**
				{"config.local", false, true},          // /config.local
				{"sub/config.local", false, false},     // anchored, not nested
				{"a/x/y/b", false, true},               // a/**/b
			},
		},
		{
			"testdata/edge-cases.gitignore",
			[]matchCase{
				{"foo bar.txt", false, true},     // spaces in filename
				{".env.local", false, true},      // .env.local
				{".env.production", false, true}, // .env.*
				{"#not-a-comment", false, true},  // \#not-a-comment
				{"test.tar.gz", false, true},     // *.tar.gz
				{"test.log.old", false, true},    // *.log.old
			},
		},
	}

	for _, f := range fixtures {
		t.Run(filepath.Base(f.path), func(t *testing.T) {
			content, err := os.ReadFile(f.path)
			if err != nil {
				t.Fatalf("read fixture %s: %v", f.path, err)
			}

			m := New()
			m.AddPatterns("", content)

			for _, tc := range f.cases {
				got := m.Match(tc.path, tc.isDir)
				if got != tc.want {
					t.Errorf("Match(%q, %v) = %v, want %v", tc.path, tc.isDir, got, tc.want)
				}
			}
		})
	}
}

// TestLoadFixtures checks that each fixture file parses into exactly the
// expected number of rules and warnings.
func TestLoadFixtures(t *testing.T) {
	fixtures := []struct {
		path     string
		rules    int
		warnings int
	}{
		{"testdata/simple.gitignore", 8, 0},
		{"testdata/complex.gitignore", 30, 0},
		{"testdata/edge-cases.gitignore", 30, 1}, // line 34: a lone "!"
		{"testdata/realistic/go.gitignore", 25, 0},
		{"testdata/realistic/node.gitignore", 33, 0},
		{"testdata/realistic/python.gitignore", 58, 0},
		{"testdata/crlf.gitignore", 5, 0},
		{"testdata/with-bom.gitignore", 6, 0},
		{"testdata/pathological.gitignore", 17, 0},
		{"testdata/realistic/large.gitignore", 655, 0},
	}

	for _, f := range fixtures {
		t.Run(filepath.Base(f.path), func(t *testing.T) {
			content, err := os.ReadFile(f.path)
			if err != nil {
				t.Fatalf("failed to read fixture %s: %v", f.path, err)
			}
			m := New()
			m.AddPatterns("", content)
			if got := m.RuleCount(); got != f.rules {
				t.Errorf("%d rules, want %d", got, f.rules)
			}
			if ws := m.Warnings(); len(ws) != f.warnings {
				t.Errorf("%d warnings, want %d: %+v", len(ws), f.warnings, ws)
			}
		})
	}
}
