package ignore_test

import (
	"cmp"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	ignore "github.com/Sriram-PR/go-ignore"
)

const parityBaselineFile = "testdata/parity/baseline.txt"

// Default scenario set. Lower parityPerSeed if a full run exceeds 60s.
var (
	paritySeeds   = []int64{1, 2, 3}
	parityPerSeed = 300
)

// TestParity compares the library with git on generated and curated
// scenarios and checks the divergences against a checked-in baseline.
// See CONTRIBUTING.md "Parity harness".
func TestParity(t *testing.T) {
	requireParity(t)
	if env := os.Getenv("GOIGNORE_PARITY_SEEDS"); env != "" {
		exploreParity(t, env)
		return
	}

	var plain, icase []scenario
	for _, s := range parityCurated() {
		if s.icase {
			icase = append(icase, s)
		} else {
			plain = append(plain, s)
		}
	}
	for _, seed := range paritySeeds {
		plain = append(plain, paritySeeded(seed, parityPerSeed, false)...)
	}
	icase = append(icase, paritySeeded(paritySeeds[0], parityPerSeed, true)...)

	var recs []string
	for _, r := range runMatchMode(t, "match", plain) {
		recs = append(recs, r.String())
	}
	for _, r := range runMatchMode(t, "match-icase", icase) {
		recs = append(recs, r.String())
	}
	for _, r := range runWalkMode(t, plain) {
		recs = append(recs, r.String())
	}
	checkBaseline(t, parityBaselineFile, recs, os.Getenv("GOIGNORE_PARITY_UPDATE") == "1")
}

// exploreParity runs match and walk on the given comma-separated seeds and
// logs every divergence without comparing against the baseline.
func exploreParity(t *testing.T, env string) {
	var n int
	for field := range strings.SplitSeq(env, ",") {
		seed, err := strconv.ParseInt(strings.TrimSpace(field), 10, 64)
		if err != nil {
			t.Fatalf("GOIGNORE_PARITY_SEEDS: %v", err)
		}
		scens := paritySeeded(seed, parityPerSeed, false)
		for _, r := range append(runMatchMode(t, "match", scens), runWalkMode(t, scens)...) {
			t.Logf("%s %s", r.scenario, r)
			n++
		}
	}
	t.Logf("explore seeds=%s divergences=%d", env, n)
}

// parityRecord is one path where git and the library disagree.
type parityRecord struct {
	scenario string // not part of String: names shift when the generator changes
	mode     string
	files    map[string]string
	path     string
	isDir    bool
	git      bool
	lib      bool
}

// String renders the record as one canonical baseline line.
func (r parityRecord) String() string {
	var b strings.Builder
	b.WriteString("{")
	for i, name := range slices.Sorted(maps.Keys(r.files)) {
		if i > 0 {
			b.WriteString(" ")
		}
		fmt.Fprintf(&b, "%q:%q", name, r.files[name])
	}
	b.WriteString("}")
	return fmt.Sprintf("%s files=%s path=%q dir=%v git=%v lib=%v", r.mode, b.String(), r.path, r.isDir, r.git, r.lib)
}

func runMatchMode(t *testing.T, mode string, scens []scenario) []parityRecord {
	t.Helper()
	var recs []parityRecord
	for _, s := range scens {
		dir, isDir := gitRepo(t, s)
		var paths []string
		for _, p := range slices.Sorted(maps.Keys(isDir)) {
			if path.Base(p) != ".gitignore" {
				paths = append(paths, p)
			}
		}
		g := gitCheckIgnore(t, s, dir, paths)
		l := libMatch(s, isDir, paths)
		for _, p := range paths {
			gv, ok := g[p]
			if !ok {
				t.Logf("%s: git did not report %q", s.name, p)
				continue
			}
			if gv != l[p] {
				recs = append(recs, parityRecord{s.name, mode, s.files, p, isDir[p], gv, l[p]})
			}
		}
	}
	return recs
}

func runWalkMode(t *testing.T, scens []scenario) []parityRecord {
	t.Helper()
	var recs []parityRecord
	for _, s := range scens {
		dir, _ := gitRepo(t, s)
		g := gitLsFiles(t, s, dir)
		l := libWalk(t, dir)
		all := maps.Clone(g)
		maps.Copy(all, l)
		for _, p := range slices.Sorted(maps.Keys(all)) {
			if g[p] != l[p] {
				recs = append(recs, parityRecord{s.name, "walk", s.files, p, false, g[p], l[p]})
			}
		}
	}
	return recs
}

// parityMatcher loads the scenario's .gitignore files shallowest first, as
// the walker does. Any order that loads a directory's file before its
// descendants' is equivalent: sibling directories' rules never apply to the
// same path.
func parityMatcher(s scenario) *ignore.Matcher {
	m := ignore.NewWithOptions(parityOptions(s))
	files := slices.SortedFunc(maps.Keys(s.files), func(a, b string) int {
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
		m.AddPatterns(base, []byte(s.files[f]))
	}
	return m
}

func parityOptions(s scenario) ignore.MatcherOptions {
	return ignore.MatcherOptions{
		CaseInsensitive: s.icase,
		MaxPatterns:     -1,
		WarningHandler:  func(ignore.ParseWarning) {},
	}
}

// libMatch reports MatchResult.Ignored for each path.
func libMatch(s scenario, isDir map[string]bool, paths []string) map[string]bool {
	m := parityMatcher(s)
	res := make(map[string]bool, len(paths))
	for _, p := range paths {
		res[p] = m.MatchWithReason(p, isDir[p]).Ignored
	}
	return res
}

// libWalk returns every non-directory entry WalkDir yields, as slash paths
// relative to dir.
func libWalk(t *testing.T, dir string) map[string]bool {
	t.Helper()
	res := map[string]bool{}
	err := ignore.New().WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		res[filepath.ToSlash(rel)] = true
		return nil
	})
	if err != nil {
		t.Fatalf("WalkDir %s: %v", dir, err)
	}
	return res
}

// diffBaseline returns records in got but not want (regressions) and in want
// but not got (resolved), each sorted.
func diffBaseline(got, want []string) (added, resolved []string) {
	g := make(map[string]bool, len(got))
	for _, r := range got {
		g[r] = true
	}
	w := make(map[string]bool, len(want))
	for _, r := range want {
		w[r] = true
	}
	for r := range g {
		if !w[r] {
			added = append(added, r)
		}
	}
	for r := range w {
		if !g[r] {
			resolved = append(resolved, r)
		}
	}
	slices.Sort(added)
	slices.Sort(resolved)
	return added, resolved
}

const parityBaselineHeader = `# Known divergences between go-ignore and git, one per line.
# Generated by: GOIGNORE_PARITY=1 GOIGNORE_PARITY_UPDATE=1 go test -run '^TestParity$' .
# Fields: mode, the scenario's .gitignore files, the path, whether it is a
# directory, and each side's verdict (ignored, or listed for walk).
# Update this file in the same commit as the change that alters it.
`

func readBaseline(file string) ([]string, error) {
	data, err := os.ReadFile(file)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var recs []string
	for line := range strings.SplitSeq(string(data), "\n") {
		if line != "" && !strings.HasPrefix(line, "#") {
			recs = append(recs, line)
		}
	}
	return recs, nil
}

func checkBaseline(t *testing.T, file string, got []string, update bool) {
	t.Helper()
	want, err := readBaseline(file)
	if err != nil {
		t.Fatal(err)
	}
	if update {
		recs := slices.Compact(slices.Sorted(slices.Values(got)))
		body := parityBaselineHeader + strings.Join(recs, "\n")
		if len(recs) > 0 {
			body += "\n"
		}
		if err := os.WriteFile(file, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Logf("baseline %s updated: %v -> %v", file, countByMode(want), countByMode(recs))
		return
	}
	added, resolved := diffBaseline(got, want)
	if len(added) == 0 && len(resolved) == 0 {
		return
	}
	var b strings.Builder
	if len(added) > 0 {
		fmt.Fprintf(&b, "regressions (new divergences): %d\n", len(added))
		for _, r := range added {
			b.WriteString("  " + r + "\n")
		}
	}
	if len(resolved) > 0 {
		fmt.Fprintf(&b, "resolved (update the baseline): %d\n", len(resolved))
		for _, r := range resolved {
			b.WriteString("  " + r + "\n")
		}
	}
	b.WriteString("if intended, rerun with GOIGNORE_PARITY_UPDATE=1 and commit " + file)
	t.Error(b.String())
}

func countByMode(recs []string) map[string]int {
	counts := map[string]int{}
	for _, r := range recs {
		mode, _, _ := strings.Cut(r, " ")
		counts[mode]++
	}
	return counts
}

func TestCheckBaselineDiff(t *testing.T) {
	got := []string{"match a", "match b", "walk c", "match a"}
	want := []string{"match b", "walk d"}
	added, resolved := diffBaseline(got, want)
	if !slices.Equal(added, []string{"match a", "walk c"}) {
		t.Errorf("added = %q, want [match a, walk c]", added)
	}
	if !slices.Equal(resolved, []string{"walk d"}) {
		t.Errorf("resolved = %q, want [walk d]", resolved)
	}
}

func TestParityScenarios_DirsExist(t *testing.T) {
	scens := append(parityCurated(), paritySeeded(1, 50, false)...)
	for _, s := range scens {
		for p := range s.paths {
			parts := strings.Split(p, "/")
			for k := 1; k < len(parts); k++ {
				if prefix := strings.Join(parts[:k], "/"); !s.paths[prefix] {
					t.Errorf("%s: prefix %q of %q is not a directory", s.name, prefix, p)
				}
			}
		}
		for f := range s.files {
			if d := path.Dir(f); d != "." && !s.paths[d] {
				t.Errorf("%s: %q holds a .gitignore but is not a directory", s.name, d)
			}
		}
	}
}
