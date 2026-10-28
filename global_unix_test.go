//go:build linux || darwin || freebsd || openbsd || netbsd || dragonfly

package ignore

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// within fails the test if f does not return within 2 s, and otherwise
// returns its error.
func within(t *testing.T, what string, f func() error) error {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- f() }()
	select {
	case err := <-done:
		return err
	case <-time.After(2 * time.Second):
		t.Fatalf("%s did not return within 2 s", what)
		return nil
	}
}

// A character device is not a regular file: reading /dev/zero would never end.
func TestAddPatternsFromFile_DevZeroRefused(t *testing.T) {
	err := within(t, "AddPatternsFromFile(/dev/zero)", func() error {
		return New().AddPatternsFromFile("", "/dev/zero")
	})
	if !errors.Is(err, ErrPatternFileSkipped) {
		t.Errorf("err = %v, want ErrPatternFileSkipped", err)
	}
}

// A FIFO named .git must not hang LoadRepo.
func TestLoadRepo_FIFOGitFileRefused(t *testing.T) {
	repo := t.TempDir()
	if err := syscall.Mkfifo(filepath.Join(repo, ".git"), 0o644); err != nil {
		t.Skipf("mkfifo: %v", err)
	}
	err := within(t, "LoadRepo", func() error {
		_, err := LoadRepo(repo, MatcherOptions{})
		return err
	})
	if !errors.Is(err, ErrPatternFileSkipped) {
		t.Errorf("err = %v, want ErrPatternFileSkipped", err)
	}
}

// A symlinked global ignore file is followed, as git does.
func TestAddPatternsFromFile_SymlinkFollowed(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "rules")
	if err := os.WriteFile(target, []byte("*.log\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	m := New()
	if err := m.AddPatternsFromFile("", link); err != nil {
		t.Fatalf("AddPatternsFromFile: %v", err)
	}
	if !m.Match("a.log", false) {
		t.Error("a.log not ignored; want the symlink's target read")
	}
}

// An unreadable info/exclude is left out, as git does: LoadRepo returns the
// matcher with the error, and its answers are git's.
func TestLoadRepo_UnreadableExcludeSkipped(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root reads mode-0 files")
	}
	isolateGitConfig(t)
	repo := t.TempDir()
	runGit(t, repo, "init", "-q")
	writeTree(t, repo, map[string]string{
		".gitignore":        "*.log\n",
		".git/info/exclude": "*.tmp\n",
		"a.log":             "",
		"b.tmp":             "",
		"c.txt":             "",
	})
	exclude := filepath.Join(repo, ".git", "info", "exclude")
	if err := os.Chmod(exclude, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(exclude, 0o644) })

	m, err := LoadRepo(repo, MatcherOptions{})
	if m == nil {
		t.Fatalf("LoadRepo returned no matcher (err %v); want one without info/exclude", err)
	}
	if !errors.Is(err, fs.ErrPermission) || !strings.Contains(err.Error(), "reading exclude file ") {
		t.Errorf("err = %v; want a permission error naming the exclude file", err)
	}
	for _, p := range []string{"a.log", "b.tmp", "c.txt"} {
		if got, want := m.Match(p, false), gitIgnores(t, repo, p); got != want {
			t.Errorf("Match(%q) = %v, git check-ignore says %v", p, got, want)
		}
	}
}
