package ignore_test

import (
	"fmt"
	"io/fs"
	"maps"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	ignore "github.com/Sriram-PR/go-ignore"
	"github.com/Sriram-PR/go-ignore/internal/paritycases"
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

	var plain, icase []paritycases.Scenario
	for _, s := range paritycases.Curated() {
		if s.ICase {
			icase = append(icase, s)
		} else {
			plain = append(plain, s)
		}
	}
	for _, seed := range paritySeeds {
		plain = append(plain, paritycases.Seeded(seed, parityPerSeed, false)...)
	}
	icase = append(icase, paritycases.Seeded(paritySeeds[0], parityPerSeed, true)...)

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
	if bad := runCheckMode(t, append(plain, icase...)); len(bad) > 0 {
		t.Errorf("check mode: %d explanations differ from git check-ignore -v:\n%s", len(bad), strings.Join(bad, "\n"))
	}
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
		scens := paritycases.Seeded(seed, parityPerSeed, false)
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

func runMatchMode(t *testing.T, mode string, scens []paritycases.Scenario) []parityRecord {
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
				t.Logf("%s: git did not report %q", s.Name, p)
				continue
			}
			if gv != l[p] {
				recs = append(recs, parityRecord{s.Name, mode, s.Files, p, isDir[p], gv, l[p]})
			}
		}
	}
	return recs
}

func runWalkMode(t *testing.T, scens []paritycases.Scenario) []parityRecord {
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
				recs = append(recs, parityRecord{s.Name, "walk", s.Files, p, false, g[p], l[p]})
			}
		}
	}
	return recs
}

// runCheckMode compares Tree.Check's explanation with git check-ignore -v.
// Where both agree whether a path is ignored, a path git reports a pattern
// for must be matched by the same file and line. Disagreements about Ignored
// are match mode's records.
func runCheckMode(t *testing.T, scens []paritycases.Scenario) []string {
	t.Helper()
	var bad []string
	for _, s := range scens {
		dir, isDir := gitRepo(t, s)
		var paths []string
		for _, p := range slices.Sorted(maps.Keys(isDir)) {
			if path.Base(p) != ".gitignore" {
				paths = append(paths, p)
			}
		}
		g := gitCheckIgnoreVerbose(t, s, dir, paths)
		tr := ignore.NewWithOptions(parityOptions(s)).Tree(dir)
		for _, p := range paths {
			gv, ok := g[p]
			if !ok {
				continue
			}
			res, err := tr.Check(p)
			if err != nil {
				bad = append(bad, fmt.Sprintf("%s path=%q: %v", s.Name, p, err))
				continue
			}
			if res.Ignored != gv.Ignored {
				continue
			}
			src := ""
			if res.Matched {
				rel, err := filepath.Rel(dir, res.Source)
				if err != nil {
					t.Fatal(err)
				}
				src = filepath.ToSlash(rel)
			}
			if src != gv.Source || res.Line != gv.Line {
				bad = append(bad, fmt.Sprintf("%s path=%q git=%s:%d lib=%s:%d", s.Name, p, gv.Source, gv.Line, src, res.Line))
			}
		}
	}
	return bad
}

// libMatch reports MatchResult.Ignored for each path.
func libMatch(s paritycases.Scenario, isDir map[string]bool, paths []string) map[string]bool {
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

const parityBaselineHeader = `# Known divergences between go-ignore and git, one per line. The remaining
# ones are documented in doc.go, "Differences from git".
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

// requireParity skips when GOIGNORE_PARITY is unset, and fails when it is set
// to anything but 1 or git is missing, so CI cannot pass without running the
// comparison.
func requireParity(t *testing.T) {
	t.Helper()
	switch v := os.Getenv("GOIGNORE_PARITY"); v {
	case "":
		t.Skip("set GOIGNORE_PARITY=1 to run the git parity harness")
	case "1":
	default:
		t.Fatalf("GOIGNORE_PARITY must be 1, got %q", v)
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Fatal("GOIGNORE_PARITY=1 but git is not on PATH")
	}
}

// gitRepo writes s into a fresh git repository and returns its directory and
// each path's type as found on disk.
func gitRepo(t *testing.T, s paritycases.Scenario) (string, map[string]bool) {
	t.Helper()
	dir := t.TempDir()
	if err := paritycases.InitRepo(dir, s); err != nil {
		t.Fatal(err)
	}
	isDir, err := paritycases.Materialize(dir, s)
	if err != nil {
		t.Fatal(err)
	}
	return dir, isDir
}

// gitCheckIgnore reports, for every path, whether git ignores it.
func gitCheckIgnore(t *testing.T, s paritycases.Scenario, dir string, paths []string) map[string]bool {
	t.Helper()
	res := make(map[string]bool, len(paths))
	for p, v := range gitCheckIgnoreVerbose(t, s, dir, paths) {
		res[p] = v.Ignored
	}
	return res
}

func gitCheckIgnoreVerbose(t *testing.T, s paritycases.Scenario, dir string, paths []string) map[string]paritycases.Verdict {
	t.Helper()
	v, err := paritycases.GitVerdicts(dir, s, paths)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func gitLsFiles(t *testing.T, s paritycases.Scenario, dir string) map[string]bool {
	t.Helper()
	files, err := paritycases.GitLsFiles(dir, s)
	if err != nil {
		t.Fatal(err)
	}
	return files
}
