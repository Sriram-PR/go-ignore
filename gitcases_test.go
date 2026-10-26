package ignore

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// TestGitCases_Basic tests basic patterns against git check-ignore
func TestGitCases_Basic(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not available")
	}

	tests := []struct {
		name       string
		gitignore  string
		paths      []string
		createDirs []string // directories to create (for dir-only patterns)
	}{
		{
			name:      "simple wildcards",
			gitignore: "*.log\n*.tmp\n",
			paths:     []string{"test.log", "debug.log", "test.tmp", "main.go", "readme.md"},
		},
		{
			name:       "directory patterns",
			gitignore:  "build/\nnode_modules/\n",
			paths:      []string{"build/output.js", "node_modules/lodash/index.js", "src/main.go"},
			createDirs: []string{"build", "node_modules/lodash"},
		},
		{
			name:      "negation",
			gitignore: "*.log\n!important.log\n",
			paths:     []string{"test.log", "important.log", "debug.log"},
		},
		{
			name:       "anchored patterns",
			gitignore:  "/root.txt\nsrc/temp\n",
			paths:      []string{"root.txt", "sub/root.txt", "src/temp", "lib/src/temp"},
			createDirs: []string{"sub", "src", "lib/src"},
		},
		{
			name:       "double star prefix",
			gitignore:  "**/logs\n**/temp\n",
			paths:      []string{"logs", "src/logs", "a/b/c/logs", "temp", "x/temp"},
			createDirs: []string{"src", "a/b/c", "x"},
		},
		{
			name:       "double star suffix",
			gitignore:  "build/**\nlogs/**\n",
			paths:      []string{"build/out.js", "build/sub/deep.js", "logs/error.log", "src/build"},
			createDirs: []string{"build/sub", "logs", "src"},
		},
		{
			name:       "double star middle",
			gitignore:  "a/**/b\nsrc/**/test\n",
			paths:      []string{"a/b", "a/x/b", "a/x/y/z/b", "src/test", "src/lib/test"},
			createDirs: []string{"a/x/y/z", "src/lib"},
		},
		{
			name:       "hidden files",
			gitignore:  ".env\n.env.*\n.cache/\n",
			paths:      []string{".env", ".env.local", ".env.production", ".cache/data", "env"},
			createDirs: []string{".cache"},
		},
		{
			name:      "character classes",
			gitignore: "[abc].txt\n*.[ch]\n[0-9]*.log\n",
			paths:     []string{"a.txt", "b.txt", "d.txt", "foo.c", "foo.h", "foo.o", "1debug.log", "debug.log"},
		},
		{
			name:      "negated character classes",
			gitignore: "[!abc].txt\n",
			paths:     []string{"a.txt", "d.txt", "z.txt"},
		},
		{
			name:      "character class ranges",
			gitignore: "[a-f].txt\n[a-z]akefile\n",
			paths:     []string{"a.txt", "f.txt", "g.txt", "makefile"},
		},
		{
			name:      "POSIX alpha class",
			gitignore: "[[:alpha:]].txt\n",
			paths:     []string{"a.txt", "Z.txt", "1.txt", "_.txt"},
		},
		{
			name:      "POSIX digit class",
			gitignore: "[[:digit:]]debug.log\n",
			paths:     []string{"0debug.log", "9debug.log", "adebug.log"},
		},
		{
			name:      "POSIX alnum class",
			gitignore: "[[:alnum:]].log\n",
			paths:     []string{"a.log", "Z.log", "5.log", "-.log"},
		},
		{
			name:      "POSIX upper and lower classes",
			gitignore: "[[:upper:]]ame\n[[:lower:]]ame\n",
			paths:     []string{"Name", "name", "1ame"},
		},
		{
			name:      "POSIX xdigit class",
			gitignore: "[[:xdigit:]].hex\n",
			paths:     []string{"0.hex", "f.hex", "F.hex", "g.hex"},
		},
		{
			name:      "POSIX class combined with literal",
			gitignore: "file[[:digit:]].txt\n",
			paths:     []string{"file0.txt", "file9.txt", "filea.txt", "file.txt"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			compareWithGit(t, tt.gitignore, tt.paths, tt.createDirs)
		})
	}
}

// TestGitCases_EdgeCases tests edge cases against git check-ignore
func TestGitCases_EdgeCases(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not available")
	}

	tests := []struct {
		name       string
		gitignore  string
		paths      []string
		createDirs []string
	}{
		{
			name:       "trailing slash normalization",
			gitignore:  "foo/\n",
			paths:      []string{"foo/bar.txt", "foo/sub/deep.txt", "foobar.txt"},
			createDirs: []string{"foo/sub"},
		},
		{
			name:       "complex negation",
			gitignore:  "logs/**\n!logs/keep/\n!logs/keep/**\n",
			paths:      []string{"logs/error.log", "logs/keep/important.log", "logs/other/file.log"},
			createDirs: []string{"logs/keep", "logs/other"},
		},
		{
			// Spec: "It is not possible to re-include a file if a parent
			// directory of that file is excluded."
			name:       "parent dir excluded blocks negation",
			gitignore:  "build/\n!build/keep.txt\n",
			paths:      []string{"build/keep.txt", "build/other.txt"},
			createDirs: []string{"build"},
		},
		{
			name:       "parent dir excluded blocks deep negation",
			gitignore:  "node_modules/\n!node_modules/important/keep.js\n",
			paths:      []string{"node_modules/important/keep.js", "node_modules/other.js"},
			createDirs: []string{"node_modules/important"},
		},
		{
			// A Windows-authored .gitignore commonly uses CRLF line endings.
			name:       "CRLF line endings",
			gitignore:  "*.log\r\n!keep.log\r\nbuild/\r\n",
			paths:      []string{"foo.log", "keep.log", "build/x.txt"},
			createDirs: []string{"build"},
		},
		{
			// UTF-8 BOM at the start of a .gitignore — common from
			// editors like Notepad on Windows.
			name:       "UTF-8 BOM prefix",
			gitignore:  "\xef\xbb\xbf*.log\nbuild/\n",
			paths:      []string{"foo.log", "build/x.txt"},
			createDirs: []string{"build"},
		},
		{
			name:      "multiple wildcards",
			gitignore: "*.min.js\n*.test.go\ntest_*.py\n",
			paths:     []string{"app.min.js", "lib.min.js", "foo_test.go", "test_bar.py", "main.go"},
		},
		{
			name:       "spaces in names",
			gitignore:  "my file.txt\nmy dir/\n",
			paths:      []string{"my file.txt", "myfile.txt", "my dir/content.txt"},
			createDirs: []string{"my dir"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			compareWithGit(t, tt.gitignore, tt.paths, tt.createDirs)
		})
	}
}

// compareWithGit creates a temporary git repo and compares our results with git check-ignore
func compareWithGit(t *testing.T, gitignoreContent string, paths []string, createDirs []string) {
	// Create temp directory
	tmpDir := t.TempDir()

	// Initialize git repo
	cmd := exec.Command("git", "init")
	cmd.Dir = tmpDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init failed: %v\n%s", err, out)
	}

	// Configure git user (required for some git versions)
	cmd = exec.Command("git", "config", "user.email", "test@test.com")
	cmd.Dir = tmpDir
	//nolint:errcheck // optional config, not all git versions require this
	cmd.Run()

	cmd = exec.Command("git", "config", "user.name", "Test")
	cmd.Dir = tmpDir
	//nolint:errcheck // optional config
	cmd.Run()

	// Create .gitignore
	gitignorePath := filepath.Join(tmpDir, ".gitignore")
	if err := os.WriteFile(gitignorePath, []byte(gitignoreContent), 0644); err != nil {
		t.Fatalf("failed to write .gitignore: %v", err)
	}

	// Create directories
	for _, dir := range createDirs {
		dirPath := filepath.Join(tmpDir, dir)
		if err := os.MkdirAll(dirPath, 0755); err != nil {
			t.Fatalf("failed to create directory %s: %v", dir, err)
		}
	}

	// Create test files (need to exist for git check-ignore to work properly)
	for _, path := range paths {
		fullPath := filepath.Join(tmpDir, path)
		dir := filepath.Dir(fullPath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("failed to create directory for %s: %v", path, err)
		}
		if err := os.WriteFile(fullPath, []byte("test"), 0644); err != nil {
			t.Fatalf("failed to create file %s: %v", path, err)
		}
	}

	// Create our matcher
	m := New()
	m.AddPatterns("", []byte(gitignoreContent))

	// Compare results for each path. Per-path t.Run gives clean failure
	// isolation when one path mismatches in a multi-path case.
	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			gitResult := gitCheckIgnore(t, tmpDir, path)

			fullPath := filepath.Join(tmpDir, path)
			info, err := os.Stat(fullPath)
			isDir := err == nil && info.IsDir()

			ourResult := m.Match(path, isDir)

			if ourResult != gitResult {
				t.Errorf("our result = %v, git result = %v\ngitignore:\n%s",
					ourResult, gitResult, gitignoreContent)
			}
		})
	}
}

// gitCheckIgnore runs git check-ignore and returns true if path is ignored
func gitCheckIgnore(t *testing.T, repoDir, path string) bool {
	cmd := exec.Command("git", "check-ignore", "-q", path)
	cmd.Dir = repoDir

	err := cmd.Run()
	if err == nil {
		return true // Exit 0 = ignored
	}

	// Exit 1 = not ignored, other = error
	if exitErr, ok := err.(*exec.ExitError); ok {
		if exitErr.ExitCode() == 1 {
			return false
		}
	}

	// For other errors, log but don't fail (git check-ignore can be finicky)
	t.Logf("git check-ignore warning for %q: %v", path, err)
	return false
}

// RepoEntries lists nested repositories as git ls-files does (issue #1).
func TestGitCases_NestedRepos(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not available")
	}
	root := t.TempDir()
	repo := filepath.Join(root, "r")
	// No empty directories: a directory with nothing yielded under it is
	// then a nested repository.
	writeTree(t, root, map[string]string{
		"r/.gitignore":    "ignored/\n",
		"r/top.txt":       "x",
		"r/a/.gitignore":  "inner.txt\n",
		"r/a/f.txt":       "x",
		"r/a/inner.txt":   "x",
		"r/b/.git":        "DIR",
		"r/b/f.txt":       "x",
		"r/d/f.txt":       "x",
		"r/h/k.txt":       "x",
		"r/h/i/f.txt":     "x",
		"r/ignored/f.txt": "x",
	})
	runGit(t, repo, "init", "-q")
	runGit(t, filepath.Join(repo, "a"), "init", "-q")
	runGit(t, filepath.Join(repo, "d"), "init", "-q", "--separate-git-dir", filepath.Join(root, "sep"))
	runGit(t, filepath.Join(repo, "h", "i"), "init", "-q")
	runGit(t, filepath.Join(repo, "ignored"), "init", "-q")

	cmd := exec.Command("git", "ls-files", "-co", "--exclude-standard", "-z")
	cmd.Dir = repo
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git ls-files: %v", err)
	}
	var want []string
	for p := range strings.SplitSeq(string(out), "\x00") {
		if p != "" {
			want = append(want, p)
		}
	}

	var walked []string
	for e, err := range RepoEntries(repo, MatcherOptions{}, WalkOptions{}) {
		if err != nil {
			t.Fatalf("RepoEntries: %v", err)
		}
		rel, _ := filepath.Rel(repo, e.Path)
		rel = filepath.ToSlash(rel)
		if e.IsDir() {
			rel += "/"
		}
		walked = append(walked, rel)
	}
	var got []string
	for i, p := range walked {
		if !strings.HasSuffix(p, "/") || i+1 == len(walked) || !strings.HasPrefix(walked[i+1], p) {
			got = append(got, p)
		}
	}

	slices.Sort(want)
	slices.Sort(got)
	if !slices.Equal(got, want) {
		t.Errorf("RepoEntries = %q\ngit ls-files = %q", got, want)
	}
}
