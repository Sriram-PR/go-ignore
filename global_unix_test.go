//go:build linux || darwin || freebsd || openbsd || netbsd || dragonfly

package ignore

import (
	"errors"
	"os"
	"path/filepath"
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
