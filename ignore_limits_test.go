package ignore

import (
	"errors"
	"fmt"
	"runtime"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

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

// Many pathological rules don't stop the rule after them from applying.
func TestMatch_ManyPathologicalRulesKeepLaterRules(t *testing.T) {
	var sb strings.Builder
	for range 50 {
		sb.WriteString("*a*a*a*a*a*a*a*a*b\n**/a/**/a/**/a/**/a/**/b\n")
	}
	sb.WriteString("*.env\n")
	m := New()
	m.AddPatterns("", []byte(sb.String()))
	path := strings.Repeat("a/", 40) + strings.Repeat("a", 40) + ".env"
	if !m.Match(path, false) {
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

// warnList keeps max warnings, then one marker carrying the overflowing
// warning's BasePath, and drops the rest; a negative max keeps everything.
func TestWarnList_Add(t *testing.T) {
	w := func(i int) ParseWarning { return ParseWarning{Line: i, BasePath: "b"} }
	l := newWarnList(2)
	for i := 1; i <= 5; i++ {
		l.add(w(i))
	}
	want := []ParseWarning{w(1), w(2), {Message: warnLimitMessage, BasePath: "b"}}
	if !slices.Equal(l.items, want) {
		t.Errorf("items = %+v, want %+v", l.items, want)
	}

	u := newWarnList(-1)
	for i := 1; i <= 5; i++ {
		u.add(w(i))
	}
	if len(u.items) != 5 || u.full {
		t.Errorf("unlimited: %d items, full=%v; want 5, false", len(u.items), u.full)
	}
}

// merge keeps one total cap, and an overflowed source gives the target its
// marker even when the kept warnings fit exactly.
func TestWarnList_Merge(t *testing.T) {
	w := func(i int) ParseWarning { return ParseWarning{Line: i} }
	marker := ParseWarning{Message: warnLimitMessage}

	src := newWarnList(3)
	for i := 1; i <= 5; i++ {
		src.add(w(i))
	}
	dst := newWarnList(3)
	dst.merge(&src)
	if want := []ParseWarning{w(1), w(2), w(3), marker}; !slices.Equal(dst.items, want) {
		t.Errorf("overflowed source: items = %+v, want %+v", dst.items, want)
	}

	a, b := newWarnList(3), newWarnList(3)
	a.add(w(1))
	a.add(w(2))
	b.add(w(3))
	b.add(w(4))
	dst = newWarnList(3)
	dst.merge(&a)
	dst.merge(&b)
	if want := []ParseWarning{w(1), w(2), w(3), marker}; !slices.Equal(dst.items, want) {
		t.Errorf("two sources: items = %+v, want %+v", dst.items, want)
	}
}
