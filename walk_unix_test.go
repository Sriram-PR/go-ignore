//go:build linux || darwin || freebsd || openbsd || netbsd || dragonfly

package ignore

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// A FIFO named .gitignore must not hang the walk (git itself blocks on it).
func TestWalkDir_FIFOGitignoreDoesNotHang(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(filepath.Join(root, "sub", ".gitignore"), 0o644); err != nil {
		t.Skipf("mkfifo: %v", err)
	}
	done := make(chan error, 1)
	var subErr error
	go func() {
		done <- New().WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if filepath.Base(p) == "sub" {
				subErr = err
			}
			return nil
		})
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("WalkDir: %v", err)
		}
		if !errors.Is(subErr, ErrPatternFileSkipped) {
			t.Errorf("sub err = %v, want ErrPatternFileSkipped", subErr)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("WalkDir blocked on a FIFO .gitignore")
	}
}

// A .gitignore symlinked to a file outside the tree is not followed.
func TestWalkDir_SymlinkedGitignoreNotFollowed(t *testing.T) {
	outside := filepath.Join(t.TempDir(), "rules")
	if err := os.WriteFile(outside, []byte("*.log\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	writeTree(t, root, map[string]string{"sub/a.log": "x"})
	if err := os.Symlink(outside, filepath.Join(root, "sub", ".gitignore")); err != nil {
		t.Fatal(err)
	}
	var subErr error
	sawLog := false
	err := New().WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		switch filepath.Base(p) {
		case "sub":
			subErr = err
		case "a.log":
			sawLog = true
		}
		return nil
	})
	if err != nil {
		t.Fatalf("WalkDir: %v", err)
	}
	if !errors.Is(subErr, ErrPatternFileSkipped) {
		t.Errorf("sub err = %v, want ErrPatternFileSkipped", subErr)
	}
	if !sawLog {
		t.Error("sub/a.log not visited; the symlink's rules must not apply")
	}
}

// Files and RepoFiles skip FIFOs, which git ls-files never lists and which
// block a consumer that opens them.
func TestFiles_SkipsFIFO(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "f.txt"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(filepath.Join(root, "pipe"), 0o644); err != nil {
		t.Skipf("mkfifo: %v", err)
	}
	for name, seq := range map[string]func(func(string, error) bool){
		"Files":     New().Files(root),
		"RepoFiles": RepoFiles(root, MatcherOptions{}),
	} {
		var got []string
		for p, err := range seq {
			if err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			got = append(got, filepath.Base(p))
		}
		if len(got) != 1 || got[0] != "f.txt" {
			t.Errorf("%s = %q, want [f.txt]", name, got)
		}
	}
}
