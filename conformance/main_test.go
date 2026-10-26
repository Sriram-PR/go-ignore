package conformance

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/Sriram-PR/go-ignore/internal/paritycases"
)

// TestMain keeps the developer's git config and global excludes away from
// the libraries that read them on their own (git-pkgs New, go-git), and drops
// inherited GIT_* variables such as a git hook's GIT_DIR.
func TestMain(m *testing.M) {
	os.Exit(runIsolated(m))
}

func runIsolated(m *testing.M) int {
	if err := paritycases.UnsetGitEnv(); err != nil {
		fmt.Fprintln(os.Stderr, "TestMain:", err)
		return 1
	}
	home, err := os.MkdirTemp("", "go-ignore-conformance-home-")
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
		if setErr := os.Setenv(k, v); setErr != nil {
			fmt.Fprintln(os.Stderr, "TestMain:", setErr)
			return 1
		}
	}
	return m.Run()
}
