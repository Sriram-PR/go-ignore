package ignore

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"
)

// AddGlobalPatterns loads the user's global gitignore file and adds its
// patterns to the matcher. The global gitignore path is resolved in order:
//
//  1. git config --global core.excludesFile (if git is available)
//  2. $XDG_CONFIG_HOME/git/ignore (if XDG_CONFIG_HOME is set)
//  3. ~/.config/git/ignore (default fallback)
//
// If the resolved file does not exist, AddGlobalPatterns returns nil (no error).
// Only real read failures are returned as errors.
//
// Patterns are added with an empty basePath (root scope), matching Git's
// behavior where global patterns apply to all paths.
//
// Thread-safe: can be called concurrently with Match.
func (m *Matcher) AddGlobalPatterns() error {
	path, err := resolveGlobalIgnorePath()
	if err != nil {
		return fmt.Errorf("resolving global gitignore path: %w", err)
	}
	if path == "" {
		return nil
	}

	content, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("reading global gitignore %s: %w", path, err)
	}

	m.AddPatterns("", content)
	return nil
}

// resolveGlobalIgnorePath determines the path to the global gitignore file.
// It tries git config first, then falls back to XDG conventions.
// Returns an empty string if no path can be determined.
func resolveGlobalIgnorePath() (string, error) {
	// Try git config --global core.excludesFile first
	path, err := gitConfigExcludesFile()
	if err != nil {
		return "", err
	}
	if path != "" {
		return path, nil
	}

	// Fall back to XDG path
	return xdgGlobalIgnorePath()
}

// gitConfigExcludesFile reads the global core.excludesFile from git config.
// Returns empty string if git is not available or the key is not set.
func gitConfigExcludesFile() (string, error) {
	cmd := exec.Command("git", "config", "--global", "core.excludesFile")
	out, err := cmd.Output()
	if err != nil {
		// git not installed, key not set, or other error — all fine, fall through
		return "", nil
	}

	path := strings.TrimSpace(string(out))
	if path == "" {
		return "", nil
	}

	return expandTilde(path)
}

// xdgGlobalIgnorePath returns the XDG-based global gitignore path.
// Uses $XDG_CONFIG_HOME/git/ignore if set, otherwise ~/.config/git/ignore.
func xdgGlobalIgnorePath() (string, error) {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "git", "ignore"), nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("determining home directory: %w", err)
	}
	return filepath.Join(home, ".config", "git", "ignore"), nil
}

// expandTilde expands ~ and ~user prefixes in a path.
func expandTilde(path string) (string, error) {
	if !strings.HasPrefix(path, "~") {
		return path, nil
	}

	// Split at first separator
	var userPart, rest string
	if i := strings.IndexByte(path, '/'); i >= 0 {
		userPart = path[:i]
		rest = path[i:]
	} else {
		userPart = path
		rest = ""
	}

	var homeDir string
	if userPart == "~" {
		// Current user
		dir, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("expanding ~: %w", err)
		}
		homeDir = dir
	} else {
		// ~otheruser
		username := userPart[1:]
		u, err := user.Lookup(username)
		if err != nil {
			return "", fmt.Errorf("expanding %s: %w", userPart, err)
		}
		homeDir = u.HomeDir
	}

	return homeDir + rest, nil
}
