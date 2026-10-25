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

func parityOptions(s paritycases.Scenario) ignore.MatcherOptions {
	return ignore.MatcherOptions{
		CaseInsensitive: s.ICase,
		MaxPatterns:     -1,
		WarningHandler:  func(ignore.ParseWarning) {},
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
