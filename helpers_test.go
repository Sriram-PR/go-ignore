package ignore

import (
	"errors"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// writeTree creates a directory tree from a map of relative path → file
// contents. Directories are created as needed. Empty contents create empty
// files; "DIR" creates a directory with no file.
func writeTree(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for rel, content := range files {
		full := filepath.Join(root, filepath.FromSlash(rel))
		if content == "DIR" {
			if err := os.MkdirAll(full, 0o755); err != nil {
				t.Fatalf("mkdir %s: %v", full, err)
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("mkdir parent of %s: %v", full, err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", full, err)
		}
	}
}

// merged combines writeTree layouts; later ones win.
func merged(layouts ...map[string]string) map[string]string {
	out := map[string]string{}
	for _, l := range layouts {
		maps.Copy(out, l)
	}
	return out
}

// gitDirFiles returns writeTree entries for a minimal git directory at the
// slash path prefix, one git's is_git_directory accepts.
func gitDirFiles(prefix string) map[string]string {
	return map[string]string{
		prefix + "/HEAD":    "ref: refs/heads/main\n",
		prefix + "/objects": "DIR",
		prefix + "/refs":    "DIR",
	}
}

// isolateGitConfig points every git config scope and the XDG directory into a
// fresh temp home and returns it. None of the config files exist yet.
func isolateGitConfig(t *testing.T) (home string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	home = t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(home, "global.cfg"))
	t.Setenv("GIT_CONFIG_SYSTEM", filepath.Join(home, "system.cfg"))
	return home
}

// gitPath formats p for a gitconfig value, where a backslash is an escape.
func gitPath(p string) string { return filepath.ToSlash(p) }

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// gitIgnores reports whether git check-ignore ignores path in repo.
func gitIgnores(t *testing.T, repo, path string) bool {
	t.Helper()
	cmd := exec.Command("git", "check-ignore", "-q", "--", path)
	cmd.Dir = repo
	err := cmd.Run()
	var exitErr *exec.ExitError
	switch {
	case err == nil:
		return true
	case errors.As(err, &exitErr) && exitErr.ExitCode() == 1:
		return false
	default:
		t.Fatalf("git check-ignore %s: %v", path, err)
		return false
	}
}

// setExcludes appends a core.excludesFile = target section to the gitconfig
// file cfg.
func setExcludes(t *testing.T, cfg, target string) {
	t.Helper()
	f, err := os.OpenFile(cfg, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.WriteString("[core]\n\texcludesFile = " + gitPath(target) + "\n"); err != nil {
		t.Fatal(err)
	}
}

// gitAvailable checks if git is installed and accessible
func gitAvailable() bool {
	_, err := exec.LookPath("git")
	return err == nil
}

func mustCheck(t *testing.T, tr *Tree, path string) MatchResult {
	t.Helper()
	res, err := tr.Check(path)
	if err != nil {
		t.Fatalf("Check(%q): unexpected error %v", path, err)
	}
	return res
}

// matchGlob matches one pattern segment against s the way the matcher does,
// so the fast path under test is the one newSegment picks.
func matchGlob(pattern, s string) bool {
	return matchSingleSegment(newSegment(pattern), s)
}
