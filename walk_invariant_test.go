package ignore_test

import (
	"maps"
	"slices"
	"strings"
	"testing"

	ignore "github.com/Sriram-PR/go-ignore"
)

// TestWalkAgreesWithMatch checks the walker, which skips Match's
// parent-directory check, against Match on the parity scenarios. FilesFS must
// yield exactly the files Match reports as not ignored, and no path under an
// ignored directory may be reported as not ignored. Needs no git.
//
// Case-insensitive scenarios are left out: there, sibling directories that
// differ only in case get each other's rules in load order, a state that
// cannot exist on a case-insensitive filesystem. On Windows, paths with a
// backslash in a name are left out too: there it is a path separator.
func TestWalkAgreesWithMatch(t *testing.T) {
	for _, s := range walkInvariantScenarios() {
		fsys := scenarioFS(s)
		m := parityMatcher(s)

		want := map[string]bool{}
		for p, f := range fsys {
			if !f.Mode.IsDir() && !m.Match(p, false) {
				want[p] = true
			}
		}
		got := map[string]bool{}
		for p, err := range ignore.NewWithOptions(parityOptions(s)).FilesFS(fsys, ".") {
			if err != nil {
				t.Fatalf("%s: FilesFS: %v", s.Name, err)
			}
			got[p] = true
		}
		if !maps.Equal(got, want) {
			t.Errorf("%s: FilesFS = %q, want %q\nfiles: %q", s.Name,
				slices.Sorted(maps.Keys(got)), slices.Sorted(maps.Keys(want)), s.Files)
		}

		for p, f := range fsys {
			parts := strings.Split(p, "/")
			for k := 1; k < len(parts); k++ {
				parent := strings.Join(parts[:k], "/")
				if m.Match(parent, true) && !m.Match(p, f.Mode.IsDir()) {
					t.Errorf("%s: %q is ignored but %q under it is not\nfiles: %q", s.Name, parent, p, s.Files)
					break
				}
			}
		}
	}
}
