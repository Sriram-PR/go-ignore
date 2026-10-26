package ignore

import (
	"runtime"
	"testing"
)

// The ignored parent decides the verdict, and Own still names the negation
// that matched the path but can't apply.
func TestExplain_DeadNegation(t *testing.T) {
	m := New()
	m.AddPatterns("", []byte("build/\n!build/keep.txt\n"))
	e := m.Explain("build/keep.txt", false)
	if e.Result != m.MatchWithReason("build/keep.txt", false) || e.Result.Rule != "build/" || e.Result.Line != 1 || !e.Result.Ignored {
		t.Errorf("Result = %+v, want MatchWithReason's: ignored by build/, line 1", e.Result)
	}
	if e.Parent != "build" {
		t.Errorf("Parent = %q, want build", e.Parent)
	}
	if e.Own.Rule != "!build/keep.txt" || e.Own.Line != 2 || !e.Own.Negated() {
		t.Errorf("Own = %+v, want the negation !build/keep.txt, line 2", e.Own)
	}
}

type explainCase struct {
	name       string
	rules      string
	opts       MatcherOptions
	path       string
	isDir      bool
	wantParent string
	wantOwn    string // Own.Rule
	wantOwnNeg bool
}

func TestExplain(t *testing.T) {
	tests := []explainCase{
		{"no ignored parent", "*.log\n!keep.log\n", MatcherOptions{}, "src/keep.log", false, "", "!keep.log", true},
		{"own verdict ignores", "build/\n*.txt\n", MatcherOptions{}, "build/keep.txt", false, "build", "*.txt", false},
		{"nested parent", "build/\n", MatcherOptions{}, "a/build/x/y.txt", false, "a/build", "", false},
		{"outermost parent", "a/\nb/\n", MatcherOptions{}, "a/b/c", false, "a", "", false},
		{"leading slash", "build/\n!build/keep.txt\n", MatcherOptions{}, "/build/keep.txt", false, "build", "!build/keep.txt", true},
		{"caller's case", "build/\n!build/keep.txt\n", MatcherOptions{CaseInsensitive: true}, "Build/Keep.TXT", false, "Build", "!build/keep.txt", true},
		{"override re-includes parent", "build/\n", MatcherOptions{Overrides: []string{"!build/"}}, "build/keep.txt", false, "", "", false},
		{"ignored dir itself", "build/\n", MatcherOptions{}, "build", true, "", "build/", false},
	}
	if runtime.GOOS == "windows" {
		tests = append(tests, explainCase{"backslashes", "build/\n!build/keep.txt\n", MatcherOptions{}, `a\build\keep.txt`, false, "a/build", "", false})
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewWithOptions(tt.opts)
			m.AddPatterns("", []byte(tt.rules))
			e := m.Explain(tt.path, tt.isDir)
			if want := m.MatchWithReason(tt.path, tt.isDir); e.Result != want {
				t.Errorf("Result = %+v, want MatchWithReason's %+v", e.Result, want)
			}
			if e.Parent != tt.wantParent || e.Own.Rule != tt.wantOwn || e.Own.Negated() != tt.wantOwnNeg {
				t.Errorf("Explain(%q) = Parent %q, Own %+v; want Parent %q, Own.Rule %q, negated %v",
					tt.path, e.Parent, e.Own, tt.wantParent, tt.wantOwn, tt.wantOwnNeg)
			}
			if e.Parent == "" && e.Own != e.Result {
				t.Errorf("Own = %+v, want Result %+v when no parent is ignored", e.Own, e.Result)
			}
		})
	}
}

// A path that matches nothing gives a zero Explanation.
func TestExplain_OutOfRange(t *testing.T) {
	m := New()
	m.AddPatterns("", []byte("*\n"))
	for _, p := range []string{"", ".", "..", "../x", "a/../.."} {
		if e := m.Explain(p, false); e != (Explanation{}) {
			t.Errorf("Explain(%q) = %+v, want zero", p, e)
		}
	}
}

// Explain allocates no more than MatchWithReason on the same path.
func TestExplain_Allocs(t *testing.T) {
	m := New()
	m.AddPatterns("", []byte("build/\n!build/keep.txt\n*.log\n"))
	for _, p := range []string{"build/keep.txt", "src/a.log"} {
		base := testing.AllocsPerRun(100, func() { _ = m.MatchWithReason(p, false) })
		got := testing.AllocsPerRun(100, func() { _ = m.Explain(p, false) })
		if got > base {
			t.Errorf("Explain(%q) allocates %v, MatchWithReason %v", p, got, base)
		}
	}
}
