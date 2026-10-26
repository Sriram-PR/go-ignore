package ignore

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/Sriram-PR/go-ignore/internal/paritycases"
)

// TestMain isolates the test binary (including package ignore_test examples)
// from the developer's real git setup: LoadRepo, WalkRepo, and the git parity
// tests would otherwise pick up a personal core.excludesFile or
// ~/.config/git/ignore. Inherited GIT_* variables (git hooks set GIT_DIR) are
// dropped, or the tests' git commands would write into that repository. Tests
// that exercise those sources override these with t.Setenv.
func TestMain(m *testing.M) {
	os.Exit(runIsolated(m))
}

func runIsolated(m *testing.M) int {
	if err := paritycases.UnsetGitEnv(); err != nil {
		fmt.Fprintln(os.Stderr, "TestMain:", err)
		return 1
	}
	home, err := os.MkdirTemp("", "go-ignore-test-home-")
	if err != nil {
		fmt.Fprintln(os.Stderr, "TestMain:", err)
		return 1
	}
	defer os.RemoveAll(home)

	missing := filepath.Join(home, "nonexistent")
	for k, v := range map[string]string{
		"HOME":              home,
		"USERPROFILE":       home,
		"XDG_CONFIG_HOME":   filepath.Join(home, ".config"),
		"GIT_CONFIG_GLOBAL": missing,
		"GIT_CONFIG_SYSTEM": missing,
	} {
		if err := os.Setenv(k, v); err != nil {
			fmt.Fprintln(os.Stderr, "TestMain:", err)
			return 1
		}
	}
	return m.Run()
}
