//go:build linux || darwin || freebsd || openbsd || netbsd || dragonfly

package ignore

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

// A symlink to a directory is not a directory to Check, as in git.
func TestTree_OSSymlinkIsAFile(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{".gitignore": "link/\n", "real": "DIR"})
	if err := os.Symlink("real", filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}
	if mustCheck(t, New().Tree(root), "link").Ignored {
		t.Error("link (a symlink to a directory) ignored by link/")
	}
}

// An unreadable .gitignore gives the answer without it, and the read error.
func TestTree_OSUnreadableGitignore(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root reads mode-0 files")
	}
	root := t.TempDir()
	writeTree(t, root, map[string]string{".gitignore": "*.log\n", "src/.gitignore": "*.tmp\n"})
	if err := os.Chmod(filepath.Join(root, "src", ".gitignore"), 0); err != nil {
		t.Fatal(err)
	}
	tr := New().Tree(root)
	res, err := tr.Check("src/x.log")
	if !res.Ignored || !errors.Is(err, fs.ErrPermission) {
		t.Errorf("src/x.log: %+v, %v; want ignored by *.log and a permission error", res, err)
	}
	if res, _ := tr.Check("src/x.tmp"); res.Ignored {
		t.Errorf("src/x.tmp: %+v, want not ignored (its .gitignore is unreadable)", res)
	}
}
