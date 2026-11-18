//go:build linux || darwin || freebsd || openbsd || netbsd || dragonfly

package ignore

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
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

// symlinkedRepo links to excludeDirRepo and returns the link.
func symlinkedRepo(t *testing.T) string {
	t.Helper()
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(excludeDirRepo(t, false), link); err != nil {
		t.Fatal(err)
	}
	return link
}

// The repo walkers follow a symlinked root, as git does when run in it, and
// still report the unreadable exclude file once, at the root as given.
func TestRepoWalkers_SymlinkedRootFollowed(t *testing.T) {
	link := symlinkedRepo(t)
	want := []string{filepath.Join(link, ".gitignore"), filepath.Join(link, "a.log"), filepath.Join(link, "sub", "x.txt")}

	t.Run("WalkRepo", func(t *testing.T) {
		var rootErrs []error
		var files []string
		err := WalkRepo(link, MatcherOptions{}, func(p string, d fs.DirEntry, err error) error {
			switch {
			case p == link:
				rootErrs = append(rootErrs, err)
			case !d.IsDir():
				files = append(files, p)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("WalkRepo: %v", err)
		}
		if len(rootErrs) != 1 || !errors.Is(rootErrs[0], ErrPatternFileSkipped) {
			t.Errorf("root callbacks = %v; want one with ErrPatternFileSkipped", rootErrs)
		}
		if !slices.Equal(files, want) {
			t.Errorf("files = %q, want %q", files, want)
		}
	})
	t.Run("RepoFiles", func(t *testing.T) {
		var errs []error
		var files []string
		for p, err := range RepoFiles(link, MatcherOptions{}) {
			if err != nil {
				errs = append(errs, err)
				continue
			}
			files = append(files, p)
		}
		if len(errs) != 1 || !errors.Is(errs[0], ErrPatternFileSkipped) {
			t.Errorf("errors = %v; want one ErrPatternFileSkipped", errs)
		}
		if !slices.Equal(files, want) {
			t.Errorf("files = %q, want %q", files, want)
		}
	})
	t.Run("RepoEntries with Start", func(t *testing.T) {
		var errs []error
		var paths []string
		for e, err := range RepoEntries(link, MatcherOptions{}, WalkOptions{Start: "sub"}) {
			if err != nil {
				errs = append(errs, err)
				continue
			}
			paths = append(paths, e.Path)
		}
		if len(errs) != 1 || !errors.Is(errs[0], ErrPatternFileSkipped) {
			t.Errorf("errors = %v; want one ErrPatternFileSkipped", errs)
		}
		if w := want[2:]; !slices.Equal(paths, w) {
			t.Errorf("paths = %q, want %q", paths, w)
		}
	})
}

// WalkDir keeps filepath.WalkDir's semantics: a symlinked root is one entry.
func TestWalkDir_SymlinkedRootNotFollowed(t *testing.T) {
	link := symlinkedRepo(t)
	var seen []string
	err := New().WalkDir(link, func(p string, d fs.DirEntry, err error) error {
		seen = append(seen, p)
		return err
	})
	if err != nil {
		t.Fatalf("WalkDir: %v", err)
	}
	if !slices.Equal(seen, []string{link}) {
		t.Errorf("visited %q, want only the link", seen)
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
