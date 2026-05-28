package ignore

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"
	"time"
)

// gitConfigTimeout bounds the `git config` subprocess used to resolve the
// global excludesFile, so a hung or unresponsive git binary cannot stall
// AddGlobalPatterns indefinitely.
const gitConfigTimeout = 5 * time.Second

// LoadRepo creates a Matcher pre-loaded with the four standard gitignore
// sources for a working tree, in git's precedence order (lowest first, so the
// last-loaded rule wins per the matcher's last-match-wins semantics):
//
//  1. The system gitignore (git config --system core.excludesFile; see AddSystemPatterns)
//  2. The user's global gitignore (resolved via git config / XDG; see AddGlobalPatterns)
//  3. <repoRoot>/.git/info/exclude (see AddExcludePatterns)
//  4. <repoRoot>/.gitignore (root scope)
//
// repoRoot is used only to locate the two on-disk files above; it is NOT
// stripped from paths passed to Match. Paths supplied to Match must be
// relative to repoRoot (e.g., "build/output.js"), matching git's own
// convention. Passing absolute paths or paths with a different prefix will
// produce surprising results.
//
// Missing files are silently skipped; only real read failures are returned.
// Nested .gitignore files under subdirectories are NOT walked — callers that
// need per-directory rules should call AddPatterns with the appropriate
// basePath after LoadRepo returns.
//
// Pass a zero-value MatcherOptions{} to accept all defaults.
func LoadRepo(repoRoot string, opts MatcherOptions) (*Matcher, error) {
	m := NewWithOptions(opts)

	if err := m.AddSystemPatterns(); err != nil {
		return nil, err
	}

	if err := m.AddGlobalPatterns(); err != nil {
		return nil, err
	}

	if err := m.AddExcludePatterns(filepath.Join(repoRoot, ".git")); err != nil {
		return nil, err
	}

	rootIgnore := filepath.Join(repoRoot, ".gitignore")
	content, err := os.ReadFile(rootIgnore)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return m, nil
		}
		return nil, fmt.Errorf("reading %s: %w", rootIgnore, err)
	}
	m.addPatternsFromSource("", content, rootIgnore)
	return m, nil
}

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
// Parse warnings are reported through the standard warning mechanism:
// via the WarningHandler callback if set, otherwise collected and available
// through Warnings().
//
// Trust model: this function trusts the file path returned by "git config"
// and reads its contents. It should only be called in environments where
// the git configuration is trusted.
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

	m.addPatternsFromSource("", content, path)
	return nil
}

// AddSystemPatterns loads patterns from the system-scope gitignore file
// (referenced by `git config --system core.excludesFile`, typically
// configured via /etc/gitconfig) and adds them to the matcher.
//
// If git is unavailable, the system config does not set core.excludesFile, or
// the referenced file does not exist, AddSystemPatterns returns nil (no error).
// Only real read failures are returned as errors.
//
// Patterns are added with an empty basePath (root scope), matching Git's
// behavior where system patterns apply to all paths.
//
// Trust model: this function trusts the file path returned by "git config
// --system". On multi-tenant systems where /etc/gitconfig is not
// administrator-controlled, callers should validate the configuration before
// invoking this method.
//
// Thread-safe: can be called concurrently with Match.
func (m *Matcher) AddSystemPatterns() error {
	path, err := gitConfigExcludesFileScoped("--system")
	if err != nil {
		return fmt.Errorf("resolving system gitignore path: %w", err)
	}
	if path == "" {
		return nil
	}

	content, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("reading system gitignore %s: %w", path, err)
	}

	m.addPatternsFromSource("", content, path)
	return nil
}

// AddPatternsFromFile reads the file at path and adds its patterns under the
// given basePath. It is equivalent to:
//
//	content, err := os.ReadFile(path)
//	if err != nil { return err }
//	m.AddPatterns(basePath, content)
//
// with one extra: the file's path is recorded so that MatchResult.Source
// identifies it for any rule that originated here.
//
// If path does not exist or cannot be read, the error is returned wrapped.
// Empty files add no rules.
//
// Thread-safe: can be called concurrently with Match.
func (m *Matcher) AddPatternsFromFile(basePath, path string) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("reading %s: %w", path, err)
	}
	m.addPatternsFromSource(basePath, content, path)
	return nil
}

// AddExcludePatterns loads patterns from the repository's .git/info/exclude
// file and adds them to the matcher. The gitDir parameter is the path to the
// .git directory (e.g., ".git" or an absolute path).
//
// If the exclude file does not exist, AddExcludePatterns returns nil (no error).
// Only real read failures are returned as errors.
//
// Patterns are added with an empty basePath (root scope), matching Git's
// behavior where exclude patterns apply to all paths.
//
// Parse warnings are reported through the standard warning mechanism:
// via the WarningHandler callback if set, otherwise collected and available
// through Warnings().
//
// Trust model: this function trusts the caller-provided gitDir path and
// reads the file at gitDir/info/exclude. Callers should ensure gitDir
// points to a trusted .git directory.
//
// Thread-safe: can be called concurrently with Match.
func (m *Matcher) AddExcludePatterns(gitDir string) error {
	path := filepath.Join(gitDir, "info", "exclude")

	content, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("reading %s: %w", path, err)
	}

	m.addPatternsFromSource("", content, path)
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

// gitConfigExcludesFile reads core.excludesFile from --global git config.
// Returns empty string if git is not available or the key is not set.
func gitConfigExcludesFile() (string, error) {
	return gitConfigExcludesFileScoped("--global")
}

// gitConfigExcludesFileScoped reads core.excludesFile from the given git config
// scope. scope is the git config selector ("--global" or "--system"); other
// values are passed through unchanged.
func gitConfigExcludesFileScoped(scope string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), gitConfigTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "config", scope, "core.excludesFile")
	out, err := cmd.Output()
	if err != nil {
		// Timeout — treat as "git unavailable" and fall through to XDG.
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return "", nil
		}
		// git not found, config key not set, or fatal git error (e.g., no
		// global/system config file on Windows) — expected, fall through.
		var exitErr *exec.ExitError
		if errors.Is(err, exec.ErrNotFound) || errors.As(err, &exitErr) {
			return "", nil
		}
		// Non-exec error (e.g., permission denied on the binary itself)
		return "", fmt.Errorf("running git config: %w", err)
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
