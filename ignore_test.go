package ignore

import (
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

func TestMatch_Basic(t *testing.T) {
	m := New()
	m.AddPatterns("", []byte("*.log\nbuild/\n!important.log\n"))

	tests := []struct {
		path  string
		isDir bool
		want  bool
	}{
		{"test.log", false, true},
		{"debug.log", false, true},
		{"important.log", false, false}, // negated
		{"main.go", false, false},
		{"build", true, true},
		{"build", false, false}, // not a dir
		{"src/test.log", false, true},
		{"src/build", true, true},
	}

	for _, tt := range tests {
		got := m.Match(tt.path, tt.isDir)
		if got != tt.want {
			t.Errorf("Match(%q, %v) = %v, want %v", tt.path, tt.isDir, got, tt.want)
		}
	}
}

func TestMatch_WindowsPaths(t *testing.T) {
	m := New()
	m.AddPatterns("", []byte("*.log\nsrc/build/\n"))

	tests := []struct {
		path  string
		isDir bool
		want  bool
	}{
		{"src\\test.log", false, true},
		{"src\\build", true, true},
		{"src\\lib\\debug.log", false, true},
	}

	for _, tt := range tests {
		got := m.Match(tt.path, tt.isDir)
		if got != tt.want {
			t.Errorf("Match(%q, %v) = %v, want %v", tt.path, tt.isDir, got, tt.want)
		}
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
		path  string
		isDir bool
		want  bool
	}{
		{"BUILD", true, true},
		{"build", true, true},
		{"Build", true, true},
		{"test.LOG", false, true},
		{"test.log", false, true},
		{"test.Log", false, true},
	}

	for _, tt := range tests {
		got := m.Match(tt.path, tt.isDir)
		if got != tt.want {
			t.Errorf("Match(%q, %v) = %v, want %v", tt.path, tt.isDir, got, tt.want)
		}
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
		path string
		want bool
	}{
		// Root patterns apply everywhere
		{"test.log", true},
		{"src/test.log", true},
		{"src/lib/test.log", true},

		// src patterns only in src/
		{"test.tmp", false},      // not in src/
		{"src/test.tmp", true},   // in src/
		{"src/keep.tmp", false},  // negated in src/
		{"src/lib/test.tmp", true}, // inherited

		// src/lib patterns only in src/lib/
		{"test.bak", false},        // not in src/lib/
		{"src/test.bak", false},    // not in src/lib/
		{"src/lib/test.bak", true}, // in src/lib/
	}

	for _, tt := range tests {
		got := m.Match(tt.path, false)
		if got != tt.want {
			t.Errorf("Match(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}

func TestMatchWithReason_Basic(t *testing.T) {
	m := New()
	m.AddPatterns("", []byte("*.log\n!important.log\nbuild/\n"))

	// Test regular ignore
	result := m.MatchWithReason("debug.log", false)
	if !result.Matched {
		t.Error("debug.log should match")
	}
	if !result.Ignored {
		t.Error("debug.log should be ignored")
	}
	if result.Rule != "*.log" {
		t.Errorf("Rule = %q, want %q", result.Rule, "*.log")
	}
	if result.Line != 1 {
		t.Errorf("Line = %d, want 1", result.Line)
	}
	if result.Negated {
		t.Error("Negated should be false")
	}

	// Test negation
	result = m.MatchWithReason("important.log", false)
	if !result.Matched {
		t.Error("important.log should match")
	}
	if result.Ignored {
		t.Error("important.log should NOT be ignored (negated)")
	}
	if result.Rule != "!important.log" {
		t.Errorf("Rule = %q, want %q", result.Rule, "!important.log")
	}
	if result.Line != 2 {
		t.Errorf("Line = %d, want 2", result.Line)
	}
	if !result.Negated {
		t.Error("Negated should be true")
	}

	// Test no match
	result = m.MatchWithReason("main.go", false)
	if result.Matched {
		t.Error("main.go should not match")
	}
	if result.Ignored {
		t.Error("main.go should not be ignored")
	}
	if result.Rule != "" {
		t.Errorf("Rule should be empty, got %q", result.Rule)
	}
	if result.Line != 0 {
		t.Errorf("Line should be 0, got %d", result.Line)
	}

	// Test directory
	result = m.MatchWithReason("build", true)
	if !result.Matched {
		t.Error("build/ should match")
	}
	if !result.Ignored {
		t.Error("build/ should be ignored")
	}
	if result.Rule != "build/" {
		t.Errorf("Rule = %q, want %q", result.Rule, "build/")
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
		go func(n int) {
			defer wg.Done()
			m.AddPatterns("", []byte("*.log\n"))
		}(i)
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
		path  string
		isDir bool
		want  bool
	}{
		{"node_modules", true, true},
		{"node_modules/lodash/index.js", false, true},
		{"vendor/lib/file.go", false, true},
		{"build/app.exe", false, true},
		{"dist/bundle.js", false, true},
		{"app.exe", false, true},
		{"debug.log", false, true},
		{"logs/error.log", false, true},
		{".idea/workspace.xml", false, true},
		{".vscode/settings.json", false, true},
		{"file.swp", false, true},
		{".DS_Store", false, true},
		{"Thumbs.db", false, true},
		{".env", false, true},
		{".env.local", false, true},
		{".env.production.local", false, true},
		{".gitkeep", false, false}, // negated
		{"src/main.go", false, false},
		{"README.md", false, false},
		{"package.json", false, false},
	}

	for _, tt := range tests {
		got := m.Match(tt.path, tt.isDir)
		if got != tt.want {
			t.Errorf("Match(%q, %v) = %v, want %v", tt.path, tt.isDir, got, tt.want)
		}
	}
}

func TestMatcher_GitDirectory(t *testing.T) {
	m := New()

	// User explicitly adds .git/
	m.AddPatterns("", []byte(".git/\n"))

	tests := []struct {
		path  string
		isDir bool
		want  bool
	}{
		{".git", true, true},
		{".git/config", false, true},
		{".git/objects/pack/pack-123.idx", false, true},
		{".gitignore", false, false}, // not .git/
		{".github/workflows/ci.yml", false, false},
	}

	for _, tt := range tests {
		got := m.Match(tt.path, tt.isDir)
		if got != tt.want {
			t.Errorf("Match(%q, %v) = %v, want %v", tt.path, tt.isDir, got, tt.want)
		}
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
		path string
		want bool
	}{
		{"logs", true},
		{"src/logs", true},
		{"a/b/c/logs", true},
		{"logs/debug.log", true},
		{"src/logs/error.log", true},
		{".cache", true},
		{"home/.cache", true},
		{"a/b", true},
		{"a/x/b", true},
		{"a/x/y/z/b", true},
		{"foo/bar", true},
		{"foo/bar/baz", true},
	}

	for _, tt := range tests {
		got := m.Match(tt.path, false)
		if got != tt.want {
			t.Errorf("Match(%q) = %v, want %v", tt.path, got, tt.want)
		}
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

func BenchmarkMatch_Simple(b *testing.B) {
	m := New()
	m.AddPatterns("", []byte("*.log\n*.tmp\nbuild/\nnode_modules/\n"))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.Match("src/main.go", false)
	}
}

// func BenchmarkMatch_DeepPath(b *testing.B) {
// 	m := New()
// 	m.AddPatterns("", []byte("**/test/**\n"))
// 	path := "a/b/c/d/e/f/g/h/i/j/test/foo.go"
// 	b.ResetTimer()
// 	for i := 0; i < b.N; i++ {
// 		m.Match(path, false)
// 	}
// }

// func BenchmarkMatch_ManyRules(b *testing.B) {
// 	m := New()
// 	// Simulate large .gitignore
// 	content := []byte(`
// *.log
// *.tmp
// *.bak
// *.swp
// *.swo
// build/
// dist/
// out/
// target/
// node_modules/
// vendor/
// .venv/
// __pycache__/
// *.pyc
// *.pyo
// *.class
// *.jar
// *.war
// *.o
// *.a
// *.so
// *.dylib
// *.exe
// *.dll
// .idea/
// .vscode/
// .eclipse/
// *.sublime-*
// .DS_Store
// Thumbs.db
// .env
// .env.*
// coverage/
// .nyc_output/
// *.min.js
// *.min.css
// *.map
// `)
// 	m.AddPatterns("", content)
// 	b.ResetTimer()
// 	for i := 0; i < b.N; i++ {
// 		m.Match("src/components/Button.tsx", false)
// 	}
// }

// func BenchmarkMatch_Concurrent(b *testing.B) {
// 	m := New()
// 	m.AddPatterns("", []byte("*.log\n**/node_modules/**\nbuild/\n"))
// 	b.RunParallel(func(pb *testing.PB) {
// 		for pb.Next() {
// 			m.Match("src/index.ts", false)
// 		}
// 	})
// }

// func BenchmarkMatchWithReason(b *testing.B) {
// 	m := New()
// 	m.AddPatterns("", []byte("*.log\n!important.log\nbuild/\n"))
// 	b.ResetTimer()
// 	for i := 0; i < b.N; i++ {
// 		m.MatchWithReason("debug.log", false)
// 	}
// }