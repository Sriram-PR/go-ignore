package ignore

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"sync"
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
