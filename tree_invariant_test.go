package ignore_test

import (
	"fmt"
	"strings"
	"testing"

	ignore "github.com/Sriram-PR/go-ignore"
)

// A Tree over a scenario answers exactly as a matcher preloaded with every
// .gitignore of the scenario, and never ignores a file the walker visits.
// Needs no git; leaves out the scenarios TestWalkAgreesWithMatch does.
func TestTree_AgreesWithPreloadedMatcherAndWalker(t *testing.T) {
	for _, s := range walkInvariantScenarios() {
		fsys := scenarioFS(s)
		pre := parityMatcher(s)
		tr := ignore.NewWithOptions(parityOptions(s)).TreeFS(fsys, ".")
		for p, f := range fsys {
			want := pre.MatchWithReason(p, f.Mode.IsDir())
			got, err := tr.Check(p)
			if err != nil || got != want {
				t.Errorf("%s: Check(%q) = %+v, %v; preloaded matcher %+v", s.Name, p, got, err, want)
			}
		}
		for p, err := range ignore.NewWithOptions(parityOptions(s)).FilesFS(fsys, ".") {
			if err != nil {
				t.Fatalf("%s: FilesFS: %v", s.Name, err)
			}
			if res, err := tr.Check(p); res.Ignored || err != nil {
				t.Errorf("%s: the walker visits %q, but Check = %+v, %v", s.Name, p, res, err)
			}
		}
	}
}

// Explain never changes a verdict: Result is MatchWithReason's (Matcher) or
// CheckIsDir's (Tree), Own is Result when no parent is ignored, and a Parent
// is an ignored strict ancestor of the path. Needs no git.
func TestExplain_AgreesWithMatch(t *testing.T) {
	for _, s := range walkInvariantScenarios() {
		fsys := scenarioFS(s)
		pre := parityMatcher(s)
		tr := ignore.NewWithOptions(parityOptions(s)).TreeFS(fsys, ".")
		for p, f := range fsys {
			isDir := f.Mode.IsDir()
			want, wantErr := tr.CheckIsDir(p, isDir)
			te, err := tr.Explain(p, isDir)
			if te.Result != want || fmt.Sprint(err) != fmt.Sprint(wantErr) {
				t.Errorf("%s: Tree.Explain(%q).Result = %+v, %v; CheckIsDir %+v, %v", s.Name, p, te.Result, err, want, wantErr)
			}
			me := pre.Explain(p, isDir)
			if mr := pre.MatchWithReason(p, isDir); me.Result != mr {
				t.Errorf("%s: Matcher.Explain(%q).Result = %+v; MatchWithReason %+v", s.Name, p, me.Result, mr)
			}
			for _, e := range []ignore.Explanation{te, me} {
				if e.Parent == "" && e.Own != e.Result {
					t.Errorf("%s: %q: Own = %+v, want Result %+v with no ignored parent", s.Name, p, e.Own, e.Result)
				}
				if e.Parent != "" && (!e.Result.Ignored || !strings.HasPrefix(p, e.Parent+"/")) {
					t.Errorf("%s: %q: Parent %q with Result %+v, want an ignored strict ancestor", s.Name, p, e.Parent, e.Result)
				}
			}
		}
	}
}
