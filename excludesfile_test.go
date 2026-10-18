package ignore

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

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
	cmd := exec.Command("git", "check-ignore", "-q", path)
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

// gitConfigGet maps git's exit status: 0 is a value (possibly empty), 1 is
// unset, anything else an error carrying git's message.
func TestGitConfigGet_Outcomes(t *testing.T) {
	home := isolateGitConfig(t)
	get := func() (string, bool, error) {
		return gitConfigGet("", "--global", "--get", "core.excludesFile")
	}

	if v, found, err := get(); err != nil || found {
		t.Errorf("unset: got (%q, %v, %v), want not found", v, found, err)
	}

	writeTree(t, home, map[string]string{"global.cfg": "[core]\n\texcludesFile =\n"})
	if v, found, err := get(); err != nil || !found || v != "" {
		t.Errorf("empty: got (%q, %v, %v), want (\"\", true, nil)", v, found, err)
	}

	writeTree(t, home, map[string]string{"global.cfg": "[core]\n\texcludesFile = ~/x.ign\n"})
	want := filepath.Join(home, "x.ign")
	if v, found, err := get(); err != nil || !found || filepath.Clean(v) != want {
		t.Errorf("tilde: got (%q, %v, %v), want %q", v, found, err, want)
	}

	writeTree(t, home, map[string]string{"global.cfg": "[core\n"})
	_, found, err := get()
	var exitErr *exec.ExitError
	if found || !errors.As(err, &exitErr) || !strings.Contains(err.Error(), "bad config") {
		t.Errorf("malformed: got (%v, %v), want an *exec.ExitError mentioning bad config", found, err)
	}
}
