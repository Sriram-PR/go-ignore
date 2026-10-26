package ignore

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// isNestedRepo follows git's is_nonbare_repository_dir for d/.git.
func TestIsNestedRepo(t *testing.T) {
	sha1 := strings.Repeat("0123456789", 4)
	sha256 := strings.Repeat("ab", 32)
	tests := []struct {
		name  string
		files map[string]string
		want  bool
	}{
		{"git dir", gitDirFiles("d/.git"), true},
		{"detached sha1 HEAD", merged(gitDirFiles("d/.git"), map[string]string{"d/.git/HEAD": sha1 + "\n"}), true},
		{"detached sha256 HEAD", merged(gitDirFiles("d/.git"), map[string]string{"d/.git/HEAD": sha256 + "\n"}), true},
		{"ref with whitespace", merged(gitDirFiles("d/.git"), map[string]string{"d/.git/HEAD": "ref: \t refs/heads/x\n"}), true},
		{"gitdir file", merged(gitDirFiles("real"), map[string]string{"d/.git": "gitdir: ../real\n"}), true},
		{"gitdir file CRLF", merged(gitDirFiles("real"), map[string]string{"d/.git": "gitdir: ../real\r\n"}), true},
		{"worktree gitdir with commondir", map[string]string{
			"d/.git":                     "gitdir: ../main/worktrees/d\n",
			"main/worktrees/d/HEAD":      "ref: refs/heads/wt\n",
			"main/worktrees/d/commondir": "../..\n",
			"main/objects":               "DIR",
			"main/refs":                  "DIR",
		}, true},

		{"worktree commondir CRLF", map[string]string{
			"d/.git":                     "gitdir: ../main/worktrees/d\n",
			"main/worktrees/d/HEAD":      "ref: refs/heads/wt\n",
			"main/worktrees/d/commondir": "../..\r\n",
			"main/objects":               "DIR",
			"main/refs":                  "DIR",
		}, true},

		{"no .git", map[string]string{"d/f": "x"}, false},
		{"empty .git dir", map[string]string{"d/.git": "DIR"}, false},
		{"no HEAD", map[string]string{"d/.git/objects": "DIR", "d/.git/refs": "DIR"}, false},
		{"empty HEAD", merged(gitDirFiles("d/.git"), map[string]string{"d/.git/HEAD": ""}), false},
		{"garbage HEAD", merged(gitDirFiles("d/.git"), map[string]string{"d/.git/HEAD": "garbage\n"}), false},
		{"short hex HEAD", merged(gitDirFiles("d/.git"), map[string]string{"d/.git/HEAD": "0123abc\n"}), false},
		{"ref outside refs/", merged(gitDirFiles("d/.git"), map[string]string{"d/.git/HEAD": "ref: heads/main\n"}), false},
		{"HEAD is a dir", merged(gitDirFiles("d/.git"), map[string]string{"d/.git/HEAD": "DIR"}), false},
		{"no objects", map[string]string{"d/.git/HEAD": "ref: refs/heads/main\n", "d/.git/refs": "DIR"}, false},
		{"no refs", map[string]string{"d/.git/HEAD": "ref: refs/heads/main\n", "d/.git/objects": "DIR"}, false},
		{"objects is a file", merged(gitDirFiles("d/.git"), map[string]string{"d/.git/objects": ""}), false},
		{"commondir trailing space", map[string]string{
			"d/.git":                     "gitdir: ../main/worktrees/d\n",
			"main/worktrees/d/HEAD":      "ref: refs/heads/wt\n",
			"main/worktrees/d/commondir": "../.. \n",
			"main/objects":               "DIR",
			"main/refs":                  "DIR",
		}, false},
		{".git file without gitdir", map[string]string{"d/.git": "junk\n"}, false},
		{"gitdir without space", merged(gitDirFiles("real"), map[string]string{"d/.git": "gitdir:../real\n"}), false},
		{"gitdir empty path", map[string]string{"d/.git": "gitdir: \n"}, false},
		{"gitdir to non-repo", map[string]string{"d/.git": "gitdir: ../nowhere\n", "nowhere/x": "x"}, false},
		{".git file over 1 MiB", merged(gitDirFiles("real"), map[string]string{"d/.git": "gitdir: ../real\n" + strings.Repeat("\n", 1<<20)}), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			writeTree(t, root, tt.files)
			if got := isNestedRepo(filepath.Join(root, "d")); got != tt.want {
				t.Errorf("isNestedRepo = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsNestedRepo_AbsoluteGitdir(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, gitDirFiles("sep"))
	writeTree(t, root, map[string]string{"d/.git": "gitdir: " + filepath.Join(root, "sep") + "\n"})
	if !isNestedRepo(filepath.Join(root, "d")) {
		t.Error("isNestedRepo = false, want true")
	}
}

func TestIsNestedRepo_Symlinks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges on Windows")
	}
	tests := []struct {
		name  string
		setup func(t *testing.T, root string)
		want  bool
	}{
		{".git symlink to a git dir", func(t *testing.T, root string) {
			writeTree(t, root, merged(gitDirFiles("real"), map[string]string{"d/f": "x"}))
			symlink(t, filepath.Join(root, "real"), filepath.Join(root, "d", ".git"))
		}, true},
		{"HEAD symlink into refs/", func(t *testing.T, root string) {
			writeTree(t, root, map[string]string{"d/.git/objects": "DIR", "d/.git/refs": "DIR"})
			symlink(t, "refs/heads/main", filepath.Join(root, "d", ".git", "HEAD"))
		}, true},
		{"HEAD symlink elsewhere", func(t *testing.T, root string) {
			writeTree(t, root, map[string]string{"d/.git/objects": "DIR", "d/.git/refs": "DIR", "d/head": "ref: refs/heads/main\n"})
			symlink(t, "../head", filepath.Join(root, "d", ".git", "HEAD"))
		}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			tt.setup(t, root)
			if got := isNestedRepo(filepath.Join(root, "d")); got != tt.want {
				t.Errorf("isNestedRepo = %v, want %v", got, tt.want)
			}
		})
	}
}

// Git counts a .git file it cannot open as a repository.
func TestIsNestedRepo_UnreadableGitFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chmod does not block reads on Windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("root reads files regardless of mode")
	}
	root := t.TempDir()
	writeTree(t, root, merged(gitDirFiles("real"), map[string]string{"d/.git": "gitdir: ../real\n"}))
	gitFile := filepath.Join(root, "d", ".git")
	if err := os.Chmod(gitFile, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(gitFile, 0o644) })
	if !isNestedRepo(filepath.Join(root, "d")) {
		t.Error("isNestedRepo = false, want true")
	}
}

func symlink(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
}
