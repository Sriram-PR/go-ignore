package ignore

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// OpenTree loads a repository's sources as WalkRepo does, plus every nested
// .gitignore on demand, and agrees with git on each.
func TestOpenTree_LoadsRepoSources(t *testing.T) {
	home := isolateGitConfig(t)
	runGit(t, home, "init", "-q", "repo")
	repo := filepath.Join(home, "repo")
	writeTree(t, home, map[string]string{
		"glob.ign":                "*.glob\n",
		"repo/.git/info/exclude":  "*.excl\n",
		"repo/.gitignore":         "*.log\n",
		"repo/src/.gitignore":     "*.tmp\n",
		"repo/src/deep/keep.file": "",
	})
	setExcludes(t, filepath.Join(home, "global.cfg"), filepath.Join(home, "glob.ign"))

	tr, err := OpenTree(repo, MatcherOptions{})
	if err != nil {
		t.Fatalf("OpenTree: %v", err)
	}
	for _, p := range []string{"a.glob", "a.excl", "a.log", "src/x.tmp", "x.tmp", "src/deep/keep.file"} {
		res := mustCheck(t, tr, p)
		if want := gitIgnores(t, repo, p); res.Ignored != want {
			t.Errorf("Check(%q).Ignored = %v, git says %v", p, res.Ignored, want)
		}
	}
	if res := mustCheck(t, tr, "a.excl"); res.Source != filepath.Join(repo, ".git", "info", "exclude") {
		t.Errorf("a.excl Source = %q, want the info/exclude path", res.Source)
	}
}

// A bad root .gitignore does not fail OpenTree; each check reports it.
func TestOpenTree_BadRootGitignorePerCheck(t *testing.T) {
	home := isolateGitConfig(t)
	runGit(t, home, "init", "-q", "repo")
	repo := filepath.Join(home, "repo")
	if err := os.Mkdir(filepath.Join(repo, ".gitignore"), 0o755); err != nil {
		t.Fatal(err)
	}
	tr, err := OpenTree(repo, MatcherOptions{})
	if err != nil {
		t.Fatalf("OpenTree: %v", err)
	}
	if _, err := tr.Check("x"); !errors.Is(err, ErrPatternFileSkipped) {
		t.Errorf("Check(x) err = %v, want ErrPatternFileSkipped", err)
	}
}
