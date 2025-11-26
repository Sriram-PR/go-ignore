package ignore

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// gitAvailable checks if git is installed and accessible
func gitAvailable() bool {
	_, err := exec.LookPath("git")
	return err == nil
}

// TestGitParity_Basic tests basic patterns against git check-ignore
func TestGitParity_Basic(t *testing.T) {
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			compareWithGit(t, tt.gitignore, tt.paths, tt.createDirs)
		})
	}
}

// TestGitParity_EdgeCases tests edge cases against git check-ignore
func TestGitParity_EdgeCases(t *testing.T) {
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
	tmpDir, err := os.MkdirTemp("", "go-ignore-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Initialize git repo
	cmd := exec.Command("git", "init")
	cmd.Dir = tmpDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init failed: %v\n%s", err, out)
	}

	// Configure git user (required for some git versions)
	cmd = exec.Command("git", "config", "user.email", "test@test.com")
	cmd.Dir = tmpDir
	_ = cmd.Run() // Ignore errors, not all git versions require this

	cmd = exec.Command("git", "config", "user.name", "Test")
	cmd.Dir = tmpDir
	_ = cmd.Run()

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

	// Compare results for each path
	for _, path := range paths {
		gitResult := gitCheckIgnore(t, tmpDir, path)

		// Determine if path is a directory
		fullPath := filepath.Join(tmpDir, path)
		info, err := os.Stat(fullPath)
		isDir := err == nil && info.IsDir()

		ourResult := m.Match(path, isDir)

		if ourResult != gitResult {
			t.Errorf("path %q: our result = %v, git result = %v\ngitignore:\n%s",
				path, ourResult, gitResult, gitignoreContent)
		}
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

// TestGitParity_Verbose runs verbose comparison for debugging
func TestGitParity_Verbose(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not available")
	}

	if testing.Short() {
		t.Skip("skipping verbose test in short mode")
	}

	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "go-ignore-verbose-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Initialize git repo
	cmd := exec.Command("git", "init")
	cmd.Dir = tmpDir
	_ = cmd.Run()

	gitignoreContent := `
# Test patterns
*.log
!important.log
build/
**/cache/
src/**/test/
`

	// Create .gitignore
	if err := os.WriteFile(filepath.Join(tmpDir, ".gitignore"), []byte(gitignoreContent), 0644); err != nil {
		t.Fatalf("failed to write .gitignore: %v", err)
	}

	// Create test structure
	dirs := []string{"build", "src/lib/test", "cache", "src/cache", "logs"}
	for _, d := range dirs {
		if err := os.MkdirAll(filepath.Join(tmpDir, d), 0755); err != nil {
			t.Fatalf("failed to create directory %s: %v", d, err)
		}
	}

	files := []string{
		"test.log", "important.log", "main.go",
		"build/output.js", "src/lib/test/spec.go",
		"cache/data.bin", "src/cache/temp.bin",
	}
	for _, f := range files {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(tmpDir, f)), 0755); err != nil {
			t.Fatalf("failed to create directory for %s: %v", f, err)
		}
		if err := os.WriteFile(filepath.Join(tmpDir, f), []byte("test"), 0644); err != nil {
			t.Fatalf("failed to write file %s: %v", f, err)
		}
	}

	// Create matcher
	m := New()
	m.AddPatterns("", []byte(gitignoreContent))

	// Check each file with verbose output
	for _, f := range files {
		info, err := os.Stat(filepath.Join(tmpDir, f))
		if err != nil {
			t.Fatalf("failed to stat file %s: %v", f, err)
		}
		isDir := info.IsDir()

		ourResult := m.MatchWithReason(f, isDir)
		gitResult := gitCheckIgnoreVerbose(tmpDir, f)

		if ourResult.Ignored != gitResult.ignored {
			t.Logf("MISMATCH %q: ours=%v (rule=%q), git=%v (rule=%q)",
				f, ourResult.Ignored, ourResult.Rule, gitResult.ignored, gitResult.rule)
		} else {
			t.Logf("MATCH    %q: ignored=%v", f, ourResult.Ignored)
		}
	}
}

type gitCheckResult struct {
	pattern string
	rule    string
	ignored bool
}

func gitCheckIgnoreVerbose(repoDir, path string) gitCheckResult {
	cmd := exec.Command("git", "check-ignore", "-v", path)
	cmd.Dir = repoDir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		return gitCheckResult{ignored: false}
	}

	// Parse output: ".gitignore:1:*.log\ttest.log"
	output := strings.TrimSpace(stdout.String())
	if output == "" {
		return gitCheckResult{ignored: false}
	}

	parts := strings.SplitN(output, "\t", 2)
	if len(parts) < 1 {
		return gitCheckResult{ignored: true}
	}

	// Extract rule from "source:line:pattern"
	ruleParts := strings.SplitN(parts[0], ":", 3)
	rule := ""
	if len(ruleParts) >= 3 {
		rule = ruleParts[2]
	}

	return gitCheckResult{ignored: true, rule: rule}
}

// TestGitParity_KnownDifferences documents known differences from git behavior
func TestGitParity_KnownDifferences(t *testing.T) {
	// Document known differences between our implementation and git
	// These are intentional simplifications or edge cases we handle differently

	differences := []struct {
		description string
		gitignore   string
		path        string
		ourBehavior string
		gitBehavior string
	}{
		{
			description: "Character classes not supported",
			gitignore:   "[abc].txt",
			path:        "a.txt",
			ourBehavior: "No match (literal [abc].txt)",
			gitBehavior: "Matches (character class)",
		},
		{
			description: "Escape sequences not fully supported",
			gitignore:   "\\!important.txt",
			path:        "!important.txt",
			ourBehavior: "Only \\# at start is supported",
			gitBehavior: "Matches literal !important.txt",
		},
	}

	t.Log("Known differences from git behavior:")
	for _, d := range differences {
		t.Logf("  - %s", d.description)
		t.Logf("    Pattern: %q, Path: %q", d.gitignore, d.path)
		t.Logf("    Our behavior: %s", d.ourBehavior)
		t.Logf("    Git behavior: %s", d.gitBehavior)
	}
}
