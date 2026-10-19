package ignore

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestNew(t *testing.T) {
	m := New()
	if m == nil {
		t.Fatal("New() returned nil")
	}
	if m.RuleCount() != 0 {
		t.Errorf("New() should have 0 rules, got %d", m.RuleCount())
	}
	if m.opts.MaxBacktrackIterations != DefaultMaxBacktrackIterations {
		t.Errorf("Default MaxBacktrackIterations = %d, want %d",
			m.opts.MaxBacktrackIterations, DefaultMaxBacktrackIterations)
	}
	if m.opts.CaseInsensitive {
		t.Error("Default CaseInsensitive should be false")
	}
}

func TestNewWithOptions(t *testing.T) {
	opts := MatcherOptions{
		MaxBacktrackIterations: 5000,
		CaseInsensitive:        true,
	}
	m := NewWithOptions(opts)
	if m == nil {
		t.Fatal("NewWithOptions() returned nil")
	}
	if m.opts.MaxBacktrackIterations != 5000 {
		t.Errorf("MaxBacktrackIterations = %d, want 5000", m.opts.MaxBacktrackIterations)
	}
	if !m.opts.CaseInsensitive {
		t.Error("CaseInsensitive should be true")
	}
}

func TestNewWithOptions_DefaultBacktrack(t *testing.T) {
	// When MaxBacktrackIterations is 0, should use default
	opts := MatcherOptions{
		MaxBacktrackIterations: 0,
	}
	m := NewWithOptions(opts)
	if m.opts.MaxBacktrackIterations != DefaultMaxBacktrackIterations {
		t.Errorf("MaxBacktrackIterations = %d, want %d (default)",
			m.opts.MaxBacktrackIterations, DefaultMaxBacktrackIterations)
	}
}

func TestAddPatterns_Basic(t *testing.T) {
	m := New()
	content := []byte("*.log\nbuild/\n")
	m.AddPatterns("", content)

	if w := m.Warnings(); len(w) != 0 {
		t.Errorf("Warnings = %d, want 0", len(w))
	}
	if m.RuleCount() != 2 {
		t.Errorf("RuleCount = %d, want 2", m.RuleCount())
	}
}

func TestAddPatterns_WithBasePath(t *testing.T) {
	m := New()
	m.AddPatterns("", []byte("*.log\n"))
	m.AddPatterns("src", []byte("*.tmp\n"))
	m.AddPatterns("src/lib", []byte("*.bak\n"))

	if m.RuleCount() != 3 {
		t.Errorf("RuleCount = %d, want 3", m.RuleCount())
	}
}

func TestAddPatterns_NilContent(t *testing.T) {
	m := New()
	m.AddPatterns("", nil)

	if w := m.Warnings(); len(w) != 0 {
		t.Errorf("Warnings = %d, want 0", len(w))
	}
	if m.RuleCount() != 0 {
		t.Errorf("RuleCount = %d, want 0", m.RuleCount())
	}
}

func TestAddPatterns_EmptyContent(t *testing.T) {
	m := New()
	m.AddPatterns("", []byte{})

	if w := m.Warnings(); len(w) != 0 {
		t.Errorf("Warnings = %d, want 0", len(w))
	}
	if m.RuleCount() != 0 {
		t.Errorf("RuleCount = %d, want 0", m.RuleCount())
	}
}

func TestAddPatternsWithSource(t *testing.T) {
	const source = "embedded://rules.gitignore"
	m := New()
	m.AddPatternsWithSource("", source, []byte("*.log\n!important.log\n"))

	r := m.MatchWithReason("debug.log", false)
	if !r.Ignored {
		t.Fatalf("debug.log should be ignored")
	}
	if r.Source != source {
		t.Errorf("Source = %q, want %q", r.Source, source)
	}

	r = m.MatchWithReason("important.log", false)
	if r.Ignored {
		t.Fatalf("important.log should be re-included")
	}
	if r.Source != source {
		t.Errorf("Source = %q, want %q (negation rule still carries source)", r.Source, source)
	}

	// Source labels must not bleed across calls: AddPatterns with no source
	// should leave Source empty even when the rule wins.
	m = New()
	m.AddPatternsWithSource("", source, []byte("a\n"))
	m.AddPatterns("", []byte("b\n"))
	if r := m.MatchWithReason("b", false); r.Source != "" {
		t.Errorf("Source = %q, want empty for AddPatterns-added rule", r.Source)
	}
	if r := m.MatchWithReason("a", false); r.Source != source {
		t.Errorf("Source = %q, want %q for AddPatternsWithSource-added rule", r.Source, source)
	}
}

func TestAddPatterns_WithWarnings(t *testing.T) {
	m := New()
	content := []byte("*.log\n!\n/\nvalid.txt\n")
	m.AddPatterns("", content)

	if w := m.Warnings(); len(w) != 2 {
		t.Errorf("Warnings = %d, want 2", len(w))
	}
	if m.RuleCount() != 2 {
		t.Errorf("RuleCount = %d, want 2 (*.log and valid.txt)", m.RuleCount())
	}
}

func TestWarnings(t *testing.T) {
	m := New()
	m.AddPatterns("", []byte("!\n"))
	m.AddPatterns("src", []byte("/\n"))

	warnings := m.Warnings()
	if len(warnings) != 2 {
		t.Errorf("Warnings() returned %d, want 2", len(warnings))
	}

	// Verify warnings are copies (mutating shouldn't affect internal state)
	if len(warnings) > 0 {
		warnings[0].Line = 999
		internal := m.Warnings()
		if internal[0].Line == 999 {
			t.Error("Warnings() should return a copy")
		}
	}
}

func TestAddPatternsReader(t *testing.T) {
	t.Run("basic", func(t *testing.T) {
		m := New()
		r := bytes.NewReader([]byte("*.log\nbuild/\n"))
		if err := m.AddPatternsReader("", r); err != nil {
			t.Fatalf("AddPatternsReader: %v", err)
		}
		if m.RuleCount() != 2 {
			t.Errorf("RuleCount = %d, want 2", m.RuleCount())
		}
		if !m.Match("debug.log", false) {
			t.Error("*.log should match debug.log")
		}
	})

	t.Run("nil reader", func(t *testing.T) {
		m := New()
		if err := m.AddPatternsReader("", nil); err != nil {
			t.Fatalf("AddPatternsReader(nil): %v", err)
		}
		if m.RuleCount() != 0 {
			t.Errorf("RuleCount = %d, want 0", m.RuleCount())
		}
	})

	t.Run("empty reader", func(t *testing.T) {
		m := New()
		if err := m.AddPatternsReader("", bytes.NewReader(nil)); err != nil {
			t.Fatalf("AddPatternsReader(empty): %v", err)
		}
		if m.RuleCount() != 0 {
			t.Errorf("RuleCount = %d, want 0", m.RuleCount())
		}
	})

	t.Run("read error wraps", func(t *testing.T) {
		m := New()
		sentinel := errors.New("simulated read failure")
		err := m.AddPatternsReader("", errReader{err: sentinel})
		if err == nil {
			t.Fatal("expected error from failing reader")
		}
		if !errors.Is(err, sentinel) {
			t.Errorf("error chain missing sentinel: %v", err)
		}
		if m.RuleCount() != 0 {
			t.Errorf("RuleCount = %d, want 0 after read failure", m.RuleCount())
		}
	})
}

type errReader struct{ err error }

func (e errReader) Read(_ []byte) (int, error) { return 0, e.err }

var _ io.Reader = errReader{}

func TestWarningHandler(t *testing.T) {
	var received []ParseWarning

	m := NewWithOptions(MatcherOptions{
		WarningHandler: func(w ParseWarning) {
			received = append(received, w)
		},
	})

	// Add patterns with warnings
	m.AddPatterns("src", []byte("!\n/\n"))

	// Handler should have received warnings
	if len(received) != 2 {
		t.Errorf("Handler received %d warnings, want 2", len(received))
	}

	// BasePath should be set on each warning
	for _, w := range received {
		if w.BasePath != "src" {
			t.Errorf("warning BasePath = %q, want %q", w.BasePath, "src")
		}
	}

	// Warnings() should be empty when handler is used
	if len(m.Warnings()) != 0 {
		t.Error("Warnings() should be empty when handler is set")
	}
}

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

func TestMatcher_Concurrent(t *testing.T) {
	m := New()
	m.AddPatterns("", []byte("*.log\n*.tmp\nbuild/\n**/cache/\n"))

	var wg sync.WaitGroup
	paths := []string{
		"test.log",
		"src/main.go",
		"build/output",
		"lib/cache/data",
		"readme.md",
	}

	// Run many concurrent Match calls
	for i := 0; i < 100; i++ {
		for _, p := range paths {
			wg.Add(1)
			go func(path string) {
				defer wg.Done()
				m.Match(path, false)
				m.MatchWithReason(path, false)
			}(p)
		}
	}

	wg.Wait()
}

func TestMatcher_ConcurrentAddAndMatch(t *testing.T) {
	m := New()

	var wg sync.WaitGroup

	// Concurrent AddPatterns
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			m.AddPatterns("", []byte("*.log\n"))
		}()
	}

	// Concurrent Match while adding
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			m.Match("test.log", false)
		}()
	}

	wg.Wait()
}

func TestMatcher_ConcurrentHandlerDispatch(t *testing.T) {
	var mu sync.Mutex
	var warnings []ParseWarning
	m := NewWithOptions(MatcherOptions{
		WarningHandler: func(w ParseWarning) {
			mu.Lock()
			warnings = append(warnings, w)
			mu.Unlock()
		},
	})

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			m.AddPatterns("", []byte("!\n")) // triggers warning
		}()
	}
	wg.Wait()

	mu.Lock()
	got := len(warnings)
	mu.Unlock()
	if got != 20 {
		t.Errorf("expected 20 warnings, got %d", got)
	}
}

// TestMatcher_HandlerReentrancy guards against the deadlock where a
// WarningHandler that calls back into AddPatterns blocks forever. The test
// runs with a hard timeout via a goroutine + channel; if the handler causes a
// deadlock the test fails rather than hangs the suite.
func TestMatcher_HandlerReentrancy(t *testing.T) {
	done := make(chan struct{})
	var calls int
	var callsMu sync.Mutex

	go func() {
		var m *Matcher
		m = NewWithOptions(MatcherOptions{
			WarningHandler: func(w ParseWarning) {
				callsMu.Lock()
				calls++
				n := calls
				callsMu.Unlock()
				// Re-enter once with a warning-producing payload; further
				// re-entries are prevented by the counter to avoid infinite
				// recursion.
				if n == 1 {
					m.AddPatterns("", []byte("!\n"))
				}
			},
		})
		m.AddPatterns("", []byte("!\n"))
		close(done)
	}()

	select {
	case <-done:
		callsMu.Lock()
		defer callsMu.Unlock()
		if calls != 2 {
			t.Errorf("expected 2 handler calls (outer + 1 reentrant), got %d", calls)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("WarningHandler reentrancy deadlocked")
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

func TestAddPatterns_MaxPatterns(t *testing.T) {
	m := NewWithOptions(MatcherOptions{MaxPatterns: 5})

	// Add 3 patterns — all should be accepted
	m.AddPatterns("", []byte("*.log\nbuild/\n*.tmp\n"))
	if w := m.Warnings(); len(w) != 0 {
		t.Errorf("unexpected warnings after first add: %v", w)
	}
	if m.RuleCount() != 3 {
		t.Fatalf("RuleCount = %d, want 3", m.RuleCount())
	}

	// Add 5 more — only 2 should be accepted (remaining capacity)
	m.AddPatterns("", []byte("a\nb\nc\nd\ne\n"))
	if m.RuleCount() != 5 {
		t.Errorf("RuleCount = %d, want 5", m.RuleCount())
	}
	w := m.Warnings()
	if len(w) != 1 {
		t.Fatalf("expected 1 truncation warning, got %d", len(w))
	}
	if w[0].Message != "maximum pattern count reached, excess patterns truncated" {
		t.Errorf("unexpected warning message: %s", w[0].Message)
	}

	// Add 1 more — should be fully skipped
	m.AddPatterns("", []byte("f\n"))
	if m.RuleCount() != 5 {
		t.Errorf("RuleCount = %d, want 5 (unchanged)", m.RuleCount())
	}
	w = m.Warnings()
	if len(w) != 2 {
		t.Fatalf("expected 2 cumulative warnings, got %d", len(w))
	}
	if w[1].Message != "maximum pattern count reached, new patterns skipped" {
		t.Errorf("unexpected warning message: %s", w[1].Message)
	}

	// Verify matching still works for admitted patterns
	if !m.Match("test.log", false) {
		t.Error("*.log should still match")
	}
	if !m.Match("build", true) {
		t.Error("build/ should still match")
	}
}

func TestAddPatterns_MaxPatternLength(t *testing.T) {
	m := NewWithOptions(MatcherOptions{MaxPatternLength: 10})

	// Short pattern (accepted) and long pattern (skipped)
	m.AddPatterns("", []byte("*.log\nthis-pattern-is-way-too-long\n"))
	if m.RuleCount() != 1 {
		t.Errorf("RuleCount = %d, want 1", m.RuleCount())
	}
	w := m.Warnings()
	if len(w) != 1 {
		t.Fatalf("expected 1 warning, got %d", len(w))
	}
	if w[0].Message != "pattern exceeds maximum length, skipped" {
		t.Errorf("unexpected warning message: %s", w[0].Message)
	}
	if !m.Match("test.log", false) {
		t.Error("*.log should match")
	}
}

func TestAddPatterns_MaxPatternsUnlimited(t *testing.T) {
	m := NewWithOptions(MatcherOptions{MaxPatterns: -1})

	// Add 1000 patterns — all should be accepted
	content := make([]byte, 0, 12*1000)
	for i := 0; i < 1000; i++ {
		content = append(content, []byte(fmt.Sprintf("pattern%d\n", i))...)
	}
	m.AddPatterns("", content)
	if w := m.Warnings(); len(w) != 0 {
		t.Errorf("unexpected warnings: %v", w)
	}
	if m.RuleCount() != 1000 {
		t.Errorf("RuleCount = %d, want 1000", m.RuleCount())
	}
}

func TestAddPatterns_MaxPatternLengthUnlimited(t *testing.T) {
	m := NewWithOptions(MatcherOptions{MaxPatternLength: -1})

	long := strings.Repeat("a", 5000)
	m.AddPatterns("", []byte(long+"\n"))
	if w := m.Warnings(); len(w) != 0 {
		t.Errorf("unexpected warnings: %v", w)
	}
	if m.RuleCount() != 1 {
		t.Errorf("RuleCount = %d, want 1", m.RuleCount())
	}
}

// Regression: with rule count >= MaxBacktrackIterations, late-rule matches
// were silently dropped because every rule consumed a budget tick on entry.
func TestMatcher_BudgetDoesNotChargePerRule(t *testing.T) {
	m := New()
	var sb strings.Builder
	for i := 0; i < DefaultMaxBacktrackIterations+1000; i++ {
		fmt.Fprintf(&sb, "neverMatch%d\n", i)
	}
	sb.WriteString("target.txt\n")
	m.AddPatterns("", []byte(sb.String()))

	if !m.Match("target.txt", false) {
		t.Fatalf("Match(target.txt) = false with %d rules; budget consumed by rule enumeration",
			m.RuleCount())
	}
}

// TestAddPatternsReaderWithSource verifies the source label is recorded on
// rules streamed from an io.Reader and surfaced via MatchResult.Source.
func TestAddPatternsReaderWithSource(t *testing.T) {
	m := New()
	if err := m.AddPatternsReaderWithSource("", "cfg://ignore", strings.NewReader("*.log\n")); err != nil {
		t.Fatalf("AddPatternsReaderWithSource: %v", err)
	}
	r := m.MatchWithReason("debug.log", false)
	if !r.Ignored {
		t.Fatal(`MatchWithReason("debug.log"): not ignored, want ignored`)
	}
	if r.Source != "cfg://ignore" {
		t.Errorf("Source = %q, want %q", r.Source, "cfg://ignore")
	}
	// A nil reader is a no-op, not an error (parity with AddPatternsReader).
	if err := m.AddPatternsReaderWithSource("", "x", nil); err != nil {
		t.Errorf("nil reader returned error: %v", err)
	}
}

// TestZeroValueMatcher verifies that a Matcher obtained without New /
// NewWithOptions (the zero value) still enforces the default resource limits
// instead of treating a zero limit literally and silently dropping every
// pattern.
func TestZeroValueMatcher(t *testing.T) {
	var m Matcher
	m.AddPatterns("", []byte("*.log\nbuild/\n"))

	if got := m.RuleCount(); got != 2 {
		t.Fatalf("zero-value Matcher RuleCount = %d, want 2 (patterns were dropped)", got)
	}
	if ws := m.Warnings(); len(ws) != 0 {
		t.Fatalf("zero-value Matcher produced %d unexpected warnings: %+v", len(ws), ws)
	}
	if !m.Match("debug.log", false) {
		t.Error(`zero-value Matcher Match("debug.log") = false, want true`)
	}
	if !m.Match("build", true) {
		t.Error(`zero-value Matcher Match("build", dir) = false, want true`)
	}
	if m.Match("src/main.go", false) {
		t.Error(`zero-value Matcher Match("src/main.go") = true, want false`)
	}
}

func BenchmarkMatch_Simple(b *testing.B) {
	b.ReportAllocs()
	m := New()
	m.AddPatterns("", []byte("*.log\n*.tmp\nbuild/\nnode_modules/\n"))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.Match("src/main.go", false)
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

type endlessReader struct{}

func (endlessReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = 'a'
	}
	return len(p), nil
}

// The reader path stops at its limit instead of reading forever. The limit
// is 8 bytes here; AddPatternsReader passes git's 100 MB.
func TestAddPatternsReader_EndlessReaderRefused(t *testing.T) {
	m := New()
	err := m.addPatternsReader("", "", endlessReader{}, 8)
	if !errors.Is(err, ErrPatternFileSkipped) {
		t.Errorf("err = %v, want ErrPatternFileSkipped", err)
	}
	if n := m.RuleCount(); n != 0 {
		t.Errorf("RuleCount = %d, want 0", n)
	}
}

// A pathological glob rule must not stop later rules from matching the same
// name, as git's answer does not depend on rule order.
func TestMatch_PathologicalGlobKeepsLaterRules(t *testing.T) {
	m := New()
	m.AddPatterns("", []byte("*a*a*a*a*a*a*a*a*b\n*.env\n"))
	name := strings.Repeat("a", 40) + ".env"
	if !m.Match(name, false) {
		t.Errorf("Match(%q) = false, want true (ignored by *.env)", name)
	}
}

// A rule's answer does not depend on the work earlier rules took: a chain of
// ** segments used to spend the shared budget and skip every later rule.
func TestMatch_DoubleStarChainKeepsLaterRules(t *testing.T) {
	path := strings.Repeat("a/", 30) + "secret/x"
	for _, later := range []string{"secret/", "x", "**/secret/**"} {
		m := New()
		m.AddPatterns("", []byte("**/a/**/b/**/c/**/d/**/e\n"+later+"\n"))
		if !m.Match(path, false) {
			t.Errorf("rules [chain, %s]: Match(%q) = false, want true", later, path)
		}
	}
}

// Patterns longer than 200 segments match, as in git.
func TestMatch_LongPatternMatches(t *testing.T) {
	path := strings.Repeat("a/", 300) + "b"
	m := New()
	m.AddPatterns("", []byte(path+"\n"))
	if !m.Match(path, false) {
		t.Errorf("a %d-segment pattern does not match its own path", 301)
	}
}

// Matching is polynomial, so many pathological rules stay fast, and the rule
// after them still applies.
func TestMatch_ManyPathologicalRulesFast(t *testing.T) {
	var sb strings.Builder
	for range 50 {
		sb.WriteString("*a*a*a*a*a*a*a*a*b\n**/a/**/a/**/a/**/a/**/b\n")
	}
	sb.WriteString("*.env\n")
	m := New()
	m.AddPatterns("", []byte(sb.String()))
	path := strings.Repeat("a/", 40) + strings.Repeat("a", 40) + ".env"
	start := time.Now()
	got := m.Match(path, false)
	if d := time.Since(start); d > 2*time.Second {
		t.Errorf("Match took %v, want well under 2s", d)
	}
	if !got {
		t.Errorf("Match(%q) = false, want true (ignored by *.env)", path)
	}
}

// The deprecated budget option is ignored: a budget of 1 used to stop the
// first ** rule, and so every rule after it.
func TestMatch_BacktrackOptionIgnored(t *testing.T) {
	m := NewWithOptions(MatcherOptions{MaxBacktrackIterations: 1})
	m.AddPatterns("", []byte("*a*a*a*a*b\n**/a/**/b\n*.log\n"))
	if !m.Match("x/aaaa/debug.log", false) {
		t.Error("x/aaaa/debug.log not ignored; *.log must apply whatever the budget option says")
	}
}

// Parsing stops at the pattern limit, so input past it costs nothing: 1 MB
// of rules with MaxPatterns 10 allocated 140 MB before.
func TestAddPatterns_StopsParsingAtLimit(t *testing.T) {
	content := []byte(strings.Repeat("a\n", 500_000))
	m := NewWithOptions(MatcherOptions{MaxPatterns: 10})
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	m.AddPatterns("", content)
	runtime.ReadMemStats(&after)
	if n := m.RuleCount(); n != 10 {
		t.Errorf("RuleCount = %d, want 10", n)
	}
	if got := after.TotalAlloc - before.TotalAlloc; got > 8<<20 {
		t.Errorf("AddPatterns allocated %d MB, want under 8 MB", got>>20)
	}
	if w := m.Warnings(); len(w) != 1 || w[0].Message != "maximum pattern count reached, excess patterns truncated" {
		t.Errorf("Warnings = %v, want one truncation warning", w)
	}
}

// Exactly as many rules as the limit allows, followed by lines that are not
// rules, is not a truncation.
func TestAddPatterns_ExactlyAtLimit(t *testing.T) {
	m := NewWithOptions(MatcherOptions{MaxPatterns: 2})
	m.AddPatterns("", []byte("a\nb\n# done\n\n"))
	if n, w := m.RuleCount(), m.Warnings(); n != 2 || len(w) != 0 {
		t.Errorf("RuleCount = %d, Warnings = %v; want 2 rules and no warning", n, w)
	}
}

// Concurrent adds share the capacity: the matcher never holds more than
// MaxPatterns rules.
func TestAddPatterns_ConcurrentRespectsLimit(t *testing.T) {
	m := NewWithOptions(MatcherOptions{MaxPatterns: 100})
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() { m.AddPatterns("", []byte(strings.Repeat("x\n", 30))) })
	}
	wg.Wait()
	if n := m.RuleCount(); n != 100 {
		t.Errorf("RuleCount = %d, want 100", n)
	}
}

// Collected warnings stop at maxRetainedWarnings plus one marker, across
// calls; a WarningHandler still receives every warning.
func TestWarnings_Capped(t *testing.T) {
	bad := []byte(strings.Repeat("!\n", maxRetainedWarnings+500))
	m := New()
	m.AddPatterns("", bad)
	m.AddPatterns("sub", bad)
	w := m.Warnings()
	if len(w) != maxRetainedWarnings+1 {
		t.Fatalf("len(Warnings) = %d, want %d", len(w), maxRetainedWarnings+1)
	}
	if last := w[len(w)-1]; last.Message != "warning limit reached, further warnings dropped" || last.BasePath != "" {
		t.Errorf("last warning = %+v, want the drop marker for the first call", last)
	}

	var n atomic.Int64
	h := NewWithOptions(MatcherOptions{WarningHandler: func(ParseWarning) { n.Add(1) }})
	h.AddPatterns("", bad)
	if got := n.Load(); got != maxRetainedWarnings+500 {
		t.Errorf("handler saw %d warnings, want %d", got, maxRetainedWarnings+500)
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
