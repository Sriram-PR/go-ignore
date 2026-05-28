package ignore

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestExpandTilde(t *testing.T) {
	t.Run("non-tilde passthrough", func(t *testing.T) {
		path, err := expandTilde("/absolute/path")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if path != "/absolute/path" {
			t.Errorf("got %q, want %q", path, "/absolute/path")
		}
	})

	t.Run("relative passthrough", func(t *testing.T) {
		path, err := expandTilde("relative/path")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if path != "relative/path" {
			t.Errorf("got %q, want %q", path, "relative/path")
		}
	})

	t.Run("tilde alone", func(t *testing.T) {
		home, err := os.UserHomeDir()
		if err != nil {
			t.Skipf("cannot get home dir: %v", err)
		}
		path, err := expandTilde("~")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if path != home {
			t.Errorf("got %q, want %q", path, home)
		}
	})

	t.Run("tilde with path", func(t *testing.T) {
		home, err := os.UserHomeDir()
		if err != nil {
			t.Skipf("cannot get home dir: %v", err)
		}
		path, err := expandTilde("~/some/path")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := home + "/some/path"
		if path != want {
			t.Errorf("got %q, want %q", path, want)
		}
	})

	t.Run("unknown user error", func(t *testing.T) {
		_, err := expandTilde("~nonexistentuserxyz123/path")
		if err == nil {
			t.Fatal("expected error for unknown user, got nil")
		}
	})
}

func TestXdgGlobalIgnorePath(t *testing.T) {
	t.Run("with XDG_CONFIG_HOME", func(t *testing.T) {
		tmp := t.TempDir()
		t.Setenv("XDG_CONFIG_HOME", tmp)

		path, err := xdgGlobalIgnorePath()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := filepath.Join(tmp, "git", "ignore")
		if path != want {
			t.Errorf("got %q, want %q", path, want)
		}
	})

	t.Run("without XDG_CONFIG_HOME", func(t *testing.T) {
		t.Setenv("XDG_CONFIG_HOME", "")

		home, err := os.UserHomeDir()
		if err != nil {
			t.Skipf("cannot get home dir: %v", err)
		}

		path, err := xdgGlobalIgnorePath()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := filepath.Join(home, ".config", "git", "ignore")
		if path != want {
			t.Errorf("got %q, want %q", path, want)
		}
	})
}

func TestGitConfigExcludesFile_Success(t *testing.T) {
	tmp := t.TempDir()

	// Create a gitignore file
	ignoreFile := filepath.Join(tmp, "my-global-ignore")
	if err := os.WriteFile(ignoreFile, []byte("*.log\nbuild/\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	// Create a .gitconfig pointing core.excludesFile at it
	gitconfig := filepath.Join(tmp, ".gitconfig")
	configContent := "[core]\n\texcludesFile = " + ignoreFile + "\n"
	if err := os.WriteFile(gitconfig, []byte(configContent), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	// Point GIT_CONFIG_GLOBAL at our fake config
	t.Setenv("GIT_CONFIG_GLOBAL", gitconfig)

	// Step 1: Verify gitConfigExcludesFile reads the path correctly
	path, err := gitConfigExcludesFile()
	if err != nil {
		t.Fatalf("gitConfigExcludesFile: %v", err)
	}
	if path == "" {
		// Git didn't return a path — likely GIT_CONFIG_GLOBAL isn't
		// supported by this git version (e.g., older git on Windows).
		t.Skip("git does not respect GIT_CONFIG_GLOBAL on this platform")
	}
	if path != ignoreFile {
		t.Errorf("gitConfigExcludesFile = %q, want %q", path, ignoreFile)
	}

	// Step 2: Verify the full AddGlobalPatterns flow loads the patterns
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmp, "nonexistent-xdg"))

	m := New()
	if err := m.AddGlobalPatterns(); err != nil {
		t.Fatalf("AddGlobalPatterns: %v", err)
	}

	if n := m.RuleCount(); n != 2 {
		t.Errorf("RuleCount = %d, want 2", n)
	}

	if !m.Match("debug.log", false) {
		t.Error("expected *.log to match debug.log")
	}
	if !m.Match("build", true) {
		t.Error("expected build/ to match build dir")
	}
	if m.Match("main.go", false) {
		t.Error("expected main.go not to match")
	}
}

func TestAddGlobalPatterns_WithXDGFile(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	// Prevent git config from interfering
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(tmp, "nonexistent-git-config"))

	gitDir := filepath.Join(tmp, "git")
	if err := os.MkdirAll(gitDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	content := []byte("*.log\nbuild/\n!important.log\n")
	if err := os.WriteFile(filepath.Join(gitDir, "ignore"), content, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	m := New()
	if err := m.AddGlobalPatterns(); err != nil {
		t.Fatalf("AddGlobalPatterns: %v", err)
	}

	if n := m.RuleCount(); n != 3 {
		t.Errorf("RuleCount = %d, want 3", n)
	}

	tests := []struct {
		path  string
		isDir bool
		want  bool
	}{
		{"debug.log", false, true},
		{"important.log", false, false},
		{"build", true, true},
		{"src/main.go", false, false},
	}
	for _, tt := range tests {
		if got := m.Match(tt.path, tt.isDir); got != tt.want {
			t.Errorf("Match(%q, %v) = %v, want %v", tt.path, tt.isDir, got, tt.want)
		}
	}
}

func TestAddGlobalPatterns_NoFile(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(tmp, "nonexistent-git-config"))

	// No git/ignore file created — should return nil with 0 rules

	m := New()
	if err := m.AddGlobalPatterns(); err != nil {
		t.Fatalf("AddGlobalPatterns: %v", err)
	}

	if n := m.RuleCount(); n != 0 {
		t.Errorf("RuleCount = %d, want 0", n)
	}
}

func TestAddGlobalPatterns_EmptyFile(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(tmp, "nonexistent-git-config"))

	gitDir := filepath.Join(tmp, "git")
	if err := os.MkdirAll(gitDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(gitDir, "ignore"), []byte{}, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	m := New()
	if err := m.AddGlobalPatterns(); err != nil {
		t.Fatalf("AddGlobalPatterns: %v", err)
	}

	if n := m.RuleCount(); n != 0 {
		t.Errorf("RuleCount = %d, want 0", n)
	}
}

func TestAddGlobalPatterns_WithWarningHandler(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(tmp, "nonexistent-git-config"))

	gitDir := filepath.Join(tmp, "git")
	if err := os.MkdirAll(gitDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	// Include a pattern that triggers a warning (trailing space is one example,
	// but a bare negation "!" is more reliable across implementations)
	content := []byte("*.log\n!\n")
	if err := os.WriteFile(filepath.Join(gitDir, "ignore"), content, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	var warnings []ParseWarning
	m := NewWithOptions(MatcherOptions{
		WarningHandler: func(w ParseWarning) {
			warnings = append(warnings, w)
		},
	})

	if err := m.AddGlobalPatterns(); err != nil {
		t.Fatalf("AddGlobalPatterns: %v", err)
	}

	if len(warnings) == 0 {
		t.Error("expected at least one warning from handler, got none")
	}
}

func TestAddExcludePatterns_WithFile(t *testing.T) {
	tmp := t.TempDir()
	infoDir := filepath.Join(tmp, "info")
	if err := os.MkdirAll(infoDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	content := []byte("*.log\nbuild/\n!important.log\n")
	if err := os.WriteFile(filepath.Join(infoDir, "exclude"), content, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	m := New()
	if err := m.AddExcludePatterns(tmp); err != nil {
		t.Fatalf("AddExcludePatterns: %v", err)
	}

	if n := m.RuleCount(); n != 3 {
		t.Errorf("RuleCount = %d, want 3", n)
	}

	tests := []struct {
		path  string
		isDir bool
		want  bool
	}{
		{"debug.log", false, true},
		{"important.log", false, false},
		{"build", true, true},
		{"src/main.go", false, false},
	}
	for _, tt := range tests {
		if got := m.Match(tt.path, tt.isDir); got != tt.want {
			t.Errorf("Match(%q, %v) = %v, want %v", tt.path, tt.isDir, got, tt.want)
		}
	}
}

func TestLoadRepo(t *testing.T) {
	// Isolate from any real global gitignore on the host.
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	repo := t.TempDir()

	// <repo>/.gitignore (highest precedence)
	if err := os.WriteFile(filepath.Join(repo, ".gitignore"),
		[]byte("*.log\n!important.log\n"), 0o644); err != nil {
		t.Fatalf("write .gitignore: %v", err)
	}

	// <repo>/.git/info/exclude (medium precedence)
	infoDir := filepath.Join(repo, ".git", "info")
	if err := os.MkdirAll(infoDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(infoDir, "exclude"),
		[]byte("scratch/\n"), 0o644); err != nil {
		t.Fatalf("write exclude: %v", err)
	}

	m, err := LoadRepo(repo, MatcherOptions{})
	if err != nil {
		t.Fatalf("LoadRepo: %v", err)
	}

	// Rules from both files should be active. (Global may or may not contribute
	// depending on host config; we assert a floor, not an exact count.)
	if m.RuleCount() < 3 {
		t.Errorf("RuleCount = %d, want >= 3", m.RuleCount())
	}

	tests := []struct {
		path  string
		isDir bool
		want  bool
	}{
		{"debug.log", false, true},      // .gitignore *.log
		{"important.log", false, false}, // .gitignore !important.log
		{"scratch", true, true},         // exclude scratch/
		{"src/main.go", false, false},   // no rule matches
	}
	for _, tt := range tests {
		if got := m.Match(tt.path, tt.isDir); got != tt.want {
			t.Errorf("Match(%q, %v) = %v, want %v", tt.path, tt.isDir, got, tt.want)
		}
	}
}

// TestMatchResult_Source verifies that MatchResult.Source identifies which
// file produced the winning rule for every loader that knows its source path:
// LoadRepo's root .gitignore, AddGlobalPatterns, and AddExcludePatterns.
// Rules added via the source-less AddPatterns API must leave Source empty.
func TestMatchResult_Source(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)

	// XDG global gitignore.
	globalDir := filepath.Join(xdg, "git")
	if err := os.MkdirAll(globalDir, 0o755); err != nil {
		t.Fatalf("mkdir global: %v", err)
	}
	globalPath := filepath.Join(globalDir, "ignore")
	if err := os.WriteFile(globalPath, []byte("*.global\n"), 0o644); err != nil {
		t.Fatalf("write global: %v", err)
	}

	repo := t.TempDir()
	rootIgnore := filepath.Join(repo, ".gitignore")
	if err := os.WriteFile(rootIgnore, []byte("*.log\n"), 0o644); err != nil {
		t.Fatalf("write .gitignore: %v", err)
	}
	infoDir := filepath.Join(repo, ".git", "info")
	if err := os.MkdirAll(infoDir, 0o755); err != nil {
		t.Fatalf("mkdir info: %v", err)
	}
	excludePath := filepath.Join(infoDir, "exclude")
	if err := os.WriteFile(excludePath, []byte("scratch/\n"), 0o644); err != nil {
		t.Fatalf("write exclude: %v", err)
	}

	m, err := LoadRepo(repo, MatcherOptions{})
	if err != nil {
		t.Fatalf("LoadRepo: %v", err)
	}
	// Source-less addition (AddPatterns) must NOT populate Source.
	m.AddPatterns("", []byte("*.adhoc\n"))

	cases := []struct {
		path       string
		isDir      bool
		wantSource string
	}{
		{"a.log", false, rootIgnore},    // from root .gitignore
		{"a.global", false, globalPath}, // from XDG global
		{"scratch", true, excludePath},  // from .git/info/exclude
		{"x.adhoc", false, ""},          // from AddPatterns — no source known
	}
	for _, tc := range cases {
		r := m.MatchWithReason(tc.path, tc.isDir)
		if !r.Matched {
			t.Errorf("Match(%q): no rule matched, expected source %q", tc.path, tc.wantSource)
			continue
		}
		if r.Source != tc.wantSource {
			t.Errorf("Match(%q): Source = %q, want %q (rule=%q)", tc.path, r.Source, tc.wantSource, r.Rule)
		}
	}
}

func TestLoadRepo_MissingFiles(t *testing.T) {
	// Isolate from any real global gitignore on the host.
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	repo := t.TempDir()
	// No .gitignore, no .git/info/exclude — should still succeed.

	m, err := LoadRepo(repo, MatcherOptions{})
	if err != nil {
		t.Fatalf("LoadRepo: %v", err)
	}
	if m == nil {
		t.Fatal("LoadRepo returned nil matcher")
	}
}

func TestAddExcludePatterns_NoFile(t *testing.T) {
	tmp := t.TempDir()
	// No info/exclude file created — should return nil with 0 rules

	m := New()
	if err := m.AddExcludePatterns(tmp); err != nil {
		t.Fatalf("AddExcludePatterns: %v", err)
	}

	if n := m.RuleCount(); n != 0 {
		t.Errorf("RuleCount = %d, want 0", n)
	}
}

func TestAddExcludePatterns_EmptyFile(t *testing.T) {
	tmp := t.TempDir()
	infoDir := filepath.Join(tmp, "info")
	if err := os.MkdirAll(infoDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(infoDir, "exclude"), []byte{}, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	m := New()
	if err := m.AddExcludePatterns(tmp); err != nil {
		t.Fatalf("AddExcludePatterns: %v", err)
	}

	if n := m.RuleCount(); n != 0 {
		t.Errorf("RuleCount = %d, want 0", n)
	}
}

func TestAddExcludePatterns_ReadPermissionError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission test not reliable on Windows")
	}

	tmp := t.TempDir()
	infoDir := filepath.Join(tmp, "info")
	if err := os.MkdirAll(infoDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	excludePath := filepath.Join(infoDir, "exclude")
	if err := os.WriteFile(excludePath, []byte("*.log\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := os.Chmod(excludePath, 0o000); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chmod(excludePath, 0o644)
	})

	m := New()
	err := m.AddExcludePatterns(tmp)
	if err == nil {
		t.Fatal("expected error for unreadable exclude file, got nil")
	}
}

func TestAddGlobalPatterns_ReadPermissionError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission test not reliable on Windows")
	}

	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(tmp, "nonexistent-git-config"))

	gitDir := filepath.Join(tmp, "git")
	if err := os.MkdirAll(gitDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	ignorePath := filepath.Join(gitDir, "ignore")
	if err := os.WriteFile(ignorePath, []byte("*.log\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	// Remove read permission
	if err := os.Chmod(ignorePath, 0o000); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chmod(ignorePath, 0o644) // restore for cleanup
	})

	m := New()
	err := m.AddGlobalPatterns()
	if err == nil {
		t.Fatal("expected error for unreadable file, got nil")
	}
}
