package ignore_test

import (
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
				t.Errorf("%s: Check(%q) = %+v, %v; preloaded matcher %+v", s.name, p, got, err, want)
			}
		}
		for p, err := range ignore.NewWithOptions(parityOptions(s)).FilesFS(fsys, ".") {
			if err != nil {
				t.Fatalf("%s: FilesFS: %v", s.name, err)
			}
			if res, err := tr.Check(p); res.Ignored || err != nil {
				t.Errorf("%s: the walker visits %q, but Check = %+v, %v", s.name, p, res, err)
			}
		}
	}
}
