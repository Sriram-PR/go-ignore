package ignore_test

import (
	"cmp"
	"io/fs"
	"iter"
	"maps"
	"path"
	"runtime"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	ignore "github.com/Sriram-PR/go-ignore"
	"github.com/Sriram-PR/go-ignore/internal/paritycases"
)

// parityMatcher loads the scenario's .gitignore files shallowest first, as
// the walker does. Any order that loads a directory's file before its
// descendants' is equivalent: sibling directories' rules never apply to the
// same path. Each rule's Source is its file's scenario path, as a Tree over
// the scenario's fs.FS reports it.
func parityMatcher(s paritycases.Scenario) *ignore.Matcher {
	m := ignore.NewWithOptions(parityOptions(s))
	files := slices.SortedFunc(maps.Keys(s.Files), func(a, b string) int {
		if c := cmp.Compare(strings.Count(a, "/"), strings.Count(b, "/")); c != 0 {
			return c
		}
		return cmp.Compare(a, b)
	})
	for _, f := range files {
		base := path.Dir(f)
		if base == "." {
			base = ""
		}
		m.AddPatternsWithSource(base, f, []byte(s.Files[f]))
	}
	return m
}

// parityOptions lifts the limits git doesn't have.
func parityOptions(s paritycases.Scenario) ignore.MatcherOptions {
	return ignore.MatcherOptions{
		CaseInsensitive:  s.ICase,
		MaxPatterns:      -1,
		MaxPatternLength: -1,
		WarningHandler:   func(ignore.ParseWarning) {},
	}
}

// walkInvariantScenarios is the parity scenario set without the
// case-insensitive scenarios TestWalkAgreesWithMatch leaves out.
func walkInvariantScenarios() []paritycases.Scenario {
	var scens []paritycases.Scenario
	for _, s := range paritycases.Curated() {
		if !s.ICase {
			scens = append(scens, s)
		}
	}
	for _, seed := range paritySeeds {
		scens = append(scens, paritycases.Seeded(seed, parityPerSeed, false)...)
	}
	return scens
}

// scenarioFS is s as an fs.FS. On Windows, paths with a backslash in a name
// are left out.
func scenarioFS(s paritycases.Scenario) fstest.MapFS {
	fsys := fstest.MapFS{}
	for p, dir := range s.Paths {
		if runtime.GOOS == "windows" && strings.Contains(p, `\`) {
			continue
		}
		if dir {
			fsys[p] = &fstest.MapFile{Mode: fs.ModeDir | 0o755}
		} else {
			fsys[p] = &fstest.MapFile{}
		}
	}
	for f, content := range s.Files {
		fsys[f] = &fstest.MapFile{Data: []byte(content)}
	}
	return fsys
}

// checkWalkAgreesWithMatch checks the walker, which skips Match's
// parent-directory check, against Match on s: FilesFS must yield exactly the
// files Match reports as not ignored, and no path under an ignored directory
// may be reported as not ignored.
func checkWalkAgreesWithMatch(t *testing.T, s paritycases.Scenario) {
	t.Helper()
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

// checkTreeAgreesWithMatcher checks that a Tree over s answers exactly as a
// matcher preloaded with every .gitignore of s, and never ignores a file the
// walker visits.
func checkTreeAgreesWithMatcher(t *testing.T, s paritycases.Scenario) {
	t.Helper()
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

// filesSet collects an iterator's paths, failing on any error.
func filesSet(t *testing.T, seq iter.Seq2[string, error]) map[string]bool {
	t.Helper()
	got := map[string]bool{}
	for p, err := range seq {
		if err != nil {
			t.Fatalf("walk: %v", err)
		}
		got[p] = true
	}
	return got
}

type visit struct {
	path  string
	isDir bool
}

// entryVisits ranges over seq, calling SkipDir on each entry in skip.
func entryVisits(t *testing.T, seq iter.Seq2[ignore.Entry, error], skip map[string]bool) []visit {
	t.Helper()
	var got []visit
	for e, err := range seq {
		if err != nil {
			t.Fatalf("Entries %s: %v", e.Path, err)
		}
		got = append(got, visit{e.Path, e.IsDir()})
		if skip[e.Path] {
			e.SkipDir()
		}
	}
	return got
}
