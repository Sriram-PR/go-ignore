package ignore

import (
	"fmt"
	"runtime"
	"strings"
	"sync"
	"testing"
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

func TestMatchWithReason_SharedBacktrackBudget(t *testing.T) {
	// Verify that all rules share a single backtrack budget per Match call.
	// With a low limit and many complex rules, the budget should be exhausted
	// across rules, not reset per rule.
	m := NewWithOptions(MatcherOptions{
		MaxBacktrackIterations: 100,
	})

	// Add many pathological patterns — each would consume significant budget
	for i := 0; i < 50; i++ {
		m.AddPatterns("", []byte("*a*a*a*a*b\n"))
	}

	// Match against a string that won't match but requires backtracking.
	// With shared budget of 100, this should terminate quickly.
	// With per-rule budget of 100 * 50 rules = 5000, it would take much longer.
	result := m.MatchWithReason("aaaaaaaaaa", false)
	if result.Ignored {
		t.Error("expected no match for pathological patterns")
	}
}

func TestAddPatterns_Basic(t *testing.T) {
	m := New()
	content := []byte("*.log\nbuild/\n")
	warnings := m.AddPatterns("", content)

	if len(warnings) != 0 {
		t.Errorf("AddPatterns returned %d warnings, want 0", len(warnings))
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
	warnings := m.AddPatterns("", nil)

	if warnings != nil {
		t.Errorf("AddPatterns(nil) should return nil warnings")
	}
	if m.RuleCount() != 0 {
		t.Errorf("RuleCount = %d, want 0", m.RuleCount())
	}
}

func TestAddPatterns_EmptyContent(t *testing.T) {
	m := New()
	warnings := m.AddPatterns("", []byte{})

	if len(warnings) != 0 {
		t.Errorf("AddPatterns([]) returned %d warnings, want 0", len(warnings))
	}
	if m.RuleCount() != 0 {
		t.Errorf("RuleCount = %d, want 0", m.RuleCount())
	}
}

func TestAddPatterns_WithWarnings(t *testing.T) {
	m := New()
	content := []byte("*.log\n!\n/\nvalid.txt\n")
	warnings := m.AddPatterns("", content)

	if len(warnings) != 2 {
		t.Errorf("AddPatterns returned %d warnings, want 2", len(warnings))
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

func TestSetWarningHandler(t *testing.T) {
	m := New()

	var received []ParseWarning
	var receivedBasePaths []string

	m.SetWarningHandler(func(basePath string, w ParseWarning) {
		received = append(received, w)
		receivedBasePaths = append(receivedBasePaths, basePath)
	})

	// Add patterns with warnings
	warnings := m.AddPatterns("src", []byte("!\n/\n"))

	// Should not return warnings when handler is set
	if warnings != nil {
		t.Errorf("AddPatterns should return nil when handler is set, got %v", warnings)
	}

	// Handler should have received warnings
	if len(received) != 2 {
		t.Errorf("Handler received %d warnings, want 2", len(received))
	}

	// Check basePath was passed correctly
	for _, bp := range receivedBasePaths {
		if bp != "src" {
			t.Errorf("Handler received basePath %q, want %q", bp, "src")
		}
	}

	// Warnings() should be empty when handler is used
	if len(m.Warnings()) != 0 {
		t.Error("Warnings() should be empty when handler is set")
	}
}

func TestSetWarningHandler_Precedence(t *testing.T) {
	m := New()

	// Add patterns without handler - warnings collected
	m.AddPatterns("", []byte("!\n"))
	if len(m.Warnings()) != 1 {
		t.Error("Warning should be collected when no handler")
	}

	// Set handler
	var handlerCalled bool
	m.SetWarningHandler(func(basePath string, w ParseWarning) {
		handlerCalled = true
	})

	// Add more patterns - should go to handler
	m.AddPatterns("", []byte("!\n"))
	if !handlerCalled {
		t.Error("Handler should be called after SetWarningHandler")
	}

	// Previous warnings still in collection
	if len(m.Warnings()) != 1 {
		t.Error("Previous warnings should still be in collection")
	}
}

func TestSetWarningHandler_Reset(t *testing.T) {
	m := New()

	// Set a handler and add patterns with warnings
	var handlerCount int
	m.SetWarningHandler(func(_ string, _ ParseWarning) {
		handlerCount++
	})
	m.AddPatterns("", []byte("!\n")) // triggers warning via handler
	if handlerCount != 1 {
		t.Fatalf("handler called %d times, want 1", handlerCount)
	}

	// Reset to collection mode
	m.SetWarningHandler(nil)

	// Add more patterns with warnings — should collect, not call handler
	m.AddPatterns("", []byte("/\n"))
	if handlerCount != 1 {
		t.Errorf("handler called after reset: got %d, want 1", handlerCount)
	}

	// Verify warnings are now collected
	w := m.Warnings()
	if len(w) != 1 {
		t.Errorf("Warnings() = %d, want 1 (from post-reset AddPatterns)", len(w))
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
			if result.Negated != tt.negated {
				t.Errorf("Negated = %v, want %v", result.Negated, tt.negated)
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
	if !result.Matched || !result.Ignored || result.Rule != "*.log" || result.Negated {
		t.Errorf("Example 1 failed: %+v", result)
	}

	// Example 2: Negation re-includes file
	result = m.MatchWithReason("important.log", false)
	if !result.Matched || result.Ignored || result.Rule != "!important.log" || !result.Negated {
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
	w := m.AddPatterns("", []byte("*.log\nbuild/\n*.tmp\n"))
	if len(w) != 0 {
		t.Errorf("unexpected warnings: %v", w)
	}
	if m.RuleCount() != 3 {
		t.Fatalf("RuleCount = %d, want 3", m.RuleCount())
	}

	// Add 5 more — only 2 should be accepted (remaining capacity)
	w = m.AddPatterns("", []byte("a\nb\nc\nd\ne\n"))
	if m.RuleCount() != 5 {
		t.Errorf("RuleCount = %d, want 5", m.RuleCount())
	}
	if len(w) != 1 {
		t.Fatalf("expected 1 truncation warning, got %d", len(w))
	}
	if w[0].Message != "maximum pattern count reached, excess patterns truncated" {
		t.Errorf("unexpected warning message: %s", w[0].Message)
	}

	// Add 1 more — should be fully skipped
	w = m.AddPatterns("", []byte("f\n"))
	if m.RuleCount() != 5 {
		t.Errorf("RuleCount = %d, want 5 (unchanged)", m.RuleCount())
	}
	if len(w) != 1 {
		t.Fatalf("expected 1 skip warning, got %d", len(w))
	}
	if w[0].Message != "maximum pattern count reached, new patterns skipped" {
		t.Errorf("unexpected warning message: %s", w[0].Message)
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
	w := m.AddPatterns("", []byte("*.log\nthis-pattern-is-way-too-long\n"))
	if m.RuleCount() != 1 {
		t.Errorf("RuleCount = %d, want 1", m.RuleCount())
	}
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
	w := m.AddPatterns("", content)
	if len(w) != 0 {
		t.Errorf("unexpected warnings: %v", w)
	}
	if m.RuleCount() != 1000 {
		t.Errorf("RuleCount = %d, want 1000", m.RuleCount())
	}
}

func TestAddPatterns_MaxPatternLengthUnlimited(t *testing.T) {
	m := NewWithOptions(MatcherOptions{MaxPatternLength: -1})

	long := strings.Repeat("a", 5000)
	w := m.AddPatterns("", []byte(long+"\n"))
	if len(w) != 0 {
		t.Errorf("unexpected warnings: %v", w)
	}
	if m.RuleCount() != 1 {
		t.Errorf("RuleCount = %d, want 1", m.RuleCount())
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
