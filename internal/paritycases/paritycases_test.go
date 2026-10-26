package paritycases

import (
	"crypto/sha256"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path"
	"slices"
	"strings"
	"testing"
)

// digest covers everything except Group, so it compares with the corpus as
// it was before groups existed.
func digest(scens []Scenario) string {
	h := sha256.New()
	for _, s := range scens {
		fmt.Fprintf(h, "%s %v\n", s.Name, s.ICase)
		for _, f := range slices.Sorted(maps.Keys(s.Files)) {
			fmt.Fprintf(h, "f %q %q\n", f, s.Files[f])
		}
		for _, p := range slices.Sorted(maps.Keys(s.Paths)) {
			fmt.Fprintf(h, "p %q %v\n", p, s.Paths[p])
		}
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}

// The move out of the parity tests must not change one scenario: this is the
// full parity set (curated, seeds 1-3 x 300, icase seed 1 x 300) as computed
// from parity_scenarios_test.go at fc3bb35.
func TestCorpusUnchanged(t *testing.T) {
	all := Curated()
	for _, seed := range []int64{1, 2, 3} {
		all = append(all, Seeded(seed, 300, false)...)
	}
	all = append(all, Seeded(1, 300, true)...)
	const want = "0f99e34f2037e2f589670b3cd93f858d9855fe7dea5ab34c2aa4f6608218e6af"
	if got := digest(all); len(all) != 1484 || got != want {
		t.Errorf("%d scenarios, digest %s; want 1484, %s", len(all), got, want)
	}
}

func TestCuratedGroups(t *testing.T) {
	for _, s := range Curated() {
		if s.ICase {
			if s.Group != GroupICase {
				t.Errorf("%s: icase case in group %q, want %q", s.Name, s.Group, GroupICase)
			}
			continue
		}
		if !slices.Contains(Groups, s.Group) {
			t.Errorf("%s: group %q is not in Groups", s.Name, s.Group)
		}
	}
	for _, s := range Seeded(1, 5, false) {
		if s.Group != "" {
			t.Errorf("%s: seeded scenario has group %q", s.Name, s.Group)
		}
	}
}

func TestScenarioDirsExist(t *testing.T) {
	for _, s := range append(Curated(), Seeded(1, 50, false)...) {
		for p := range s.Paths {
			parts := strings.Split(p, "/")
			for k := 1; k < len(parts); k++ {
				if prefix := strings.Join(parts[:k], "/"); !s.Paths[prefix] {
					t.Errorf("%s: prefix %q of %q is not a directory", s.Name, prefix, p)
				}
			}
		}
		for f := range s.Files {
			if d := path.Dir(f); d != "." && !s.Paths[d] {
				t.Errorf("%s: %q holds a .gitignore but is not a directory", s.Name, d)
			}
		}
	}
}

func TestGitOracle(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not on PATH")
	}
	s := Scenario{
		Name:  "oracle",
		Files: map[string]string{".gitignore": "*.log\n!keep.log\n"},
		Paths: map[string]bool{"a.log": false, "keep.log": false, "b": false},
	}
	dir := t.TempDir()
	if err := InitRepo(dir, s); err != nil {
		t.Fatal(err)
	}
	isDir, err := Materialize(dir, s)
	if err != nil {
		t.Fatal(err)
	}
	if !maps.Equal(isDir, s.Paths) {
		t.Errorf("Materialize isDir = %v, want %v", isDir, s.Paths)
	}
	got, err := GitVerdicts(dir, s, []string{"a.log", "b", "keep.log"})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]Verdict{
		"a.log":    {Source: ".gitignore", Line: 1, Ignored: true},
		"keep.log": {Source: ".gitignore", Line: 2},
		"b":        {},
	}
	if !maps.Equal(got, want) {
		t.Errorf("GitVerdicts = %+v, want %+v", got, want)
	}
	files, err := GitLsFiles(dir, s)
	if err != nil {
		t.Fatal(err)
	}
	if want := map[string]bool{".gitignore": true, "b": true, "keep.log": true}; !maps.Equal(files, want) {
		t.Errorf("GitLsFiles = %v, want %v", files, want)
	}
}

// Git hooks export GIT_DIR and friends; inherited, they would point the
// oracle and the tests at another repository.
func TestGitEnvDropsInheritedGitVars(t *testing.T) {
	t.Setenv("GIT_DIR", "/elsewhere/.git")
	t.Setenv("GIT_CONFIG_COUNT", "1")
	t.Setenv("GIT_CONFIG_KEY_0", "core.excludesFile")
	t.Setenv("git_work_tree", "/elsewhere")
	for _, kv := range gitEnv() {
		k, _, _ := strings.Cut(kv, "=")
		if isGitVar(k) && k != "GIT_CONFIG_GLOBAL" && k != "GIT_CONFIG_SYSTEM" {
			t.Errorf("gitEnv keeps %s", kv)
		}
	}

	if err := UnsetGitEnv(); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"GIT_DIR", "GIT_CONFIG_COUNT", "GIT_CONFIG_KEY_0", "git_work_tree"} {
		if v, ok := os.LookupEnv(k); ok {
			t.Errorf("UnsetGitEnv left %s=%s", k, v)
		}
	}
}
