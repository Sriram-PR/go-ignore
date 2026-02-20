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

	m := New()
	var warnings []ParseWarning
	m.SetWarningHandler(func(_ string, w ParseWarning) {
		warnings = append(warnings, w)
	})

	if err := m.AddGlobalPatterns(); err != nil {
		t.Fatalf("AddGlobalPatterns: %v", err)
	}

	if len(warnings) == 0 {
		t.Error("expected at least one warning from handler, got none")
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
