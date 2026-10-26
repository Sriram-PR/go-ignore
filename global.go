package ignore

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// gitConfigTimeout bounds each `git config` subprocess, so a hung or
// unresponsive git binary cannot stall a loader indefinitely. A var so tests
// can shorten it.
var gitConfigTimeout = 5 * time.Second

// LoadRepo creates a Matcher pre-loaded with the three standard gitignore
// sources for a working tree, in git's precedence order (lowest first, so the
// last-loaded rule wins per the matcher's last-match-wins semantics):
//
//  1. The file named by core.excludesFile, resolved as git does for repoRoot:
//     the last value across every config scope (system, global, the
//     repository's own config, -c and GIT_CONFIG_*), with include and
//     includeIf followed. When no scope sets it, $XDG_CONFIG_HOME/git/ignore
//     (default ~/.config/git/ignore, none without a home directory); a value
//     set to the empty string loads nothing, and a relative value is read
//     from repoRoot.
//  2. <repoRoot>/.git/info/exclude, following a .git file in linked worktrees
//     and submodules (see AddExcludePatterns)
//  3. <repoRoot>/.gitignore (root scope)
//
// The value comes from running "git config" in repoRoot, with GIT_DIR and
// GIT_WORK_TREE removed from its environment. If git is not installed or does
// not answer within 5 seconds, the XDG default is used. If git rejects the
// repository (a malformed .git file or .git/config, say), the value is read
// without the repository's own config. Any other git config failure, such as a malformed
// system or global gitconfig, is returned as an error.
//
// repoRoot locates the repository and the two on-disk files above; it is NOT
// stripped from paths passed to Match. Paths supplied to Match must be
// relative to repoRoot (e.g., "build/output.js"), matching git's own
// convention. Passing absolute paths or paths with a different prefix will
// produce surprising results.
//
// Missing files are silently skipped; only real read failures are returned.
// Every file must be a regular file of at most 100 MB, and the root
// .gitignore must not be a symlink (git does not follow one); otherwise the
// error wraps ErrPatternFileSkipped.
//
// With MatcherOptions.IgnoreFiles, the root's extra ignore files are read
// right after the root .gitignore, under the same rules.
//
// Nested .gitignore files under subdirectories are NOT loaded, so Match is
// wrong for paths they govern. To check single paths with them, use OpenTree;
// to visit a whole tree, WalkRepo or WalkDir (both read each directory's
// .gitignore as they descend).
//
// Trust model: the repository's own config can choose the excludes file, as
// in git. git skips that config when the repository is owned by another user
// (safe.directory); the file is only read as patterns, under the limits
// above.
//
// Pass a zero-value MatcherOptions{} to accept all defaults.
func LoadRepo(repoRoot string, opts MatcherOptions) (*Matcher, error) {
	m, err := loadRepoConfig(repoRoot, opts)
	if err != nil {
		return nil, err
	}

	files, err := readIgnoreFiles(osSource{}, filepath.Join, repoRoot, m.opts.IgnoreFiles, nil)
	if err != nil {
		return nil, err
	}
	for _, f := range files {
		m.addPatternsFromSource("", f.content, f.path)
	}
	return m, nil
}

// loadRepoConfig loads LoadRepo's sources except the root .gitignore:
// core.excludesFile and info/exclude. WalkRepo and RepoFiles leave the
// root .gitignore to the walker, which reports a bad one through fn.
func loadRepoConfig(repoRoot string, opts MatcherOptions) (*Matcher, error) {
	m := NewWithOptions(opts)
	path, err := effectiveExcludesFile(repoRoot)
	if err != nil {
		return nil, err
	}
	// git reads a relative value from the top of the work tree.
	if path != "" {
		path = resolveRelative(repoRoot, path)
	}
	if err := m.addExcludesFile(path, "core.excludesFile"); err != nil {
		return nil, err
	}
	if err := m.AddExcludePatterns(filepath.Join(repoRoot, ".git")); err != nil {
		return nil, err
	}
	return m, nil
}

// AddGlobalPatterns loads the user's global gitignore file and adds its
// patterns to the matcher. The path is core.excludesFile from the global git
// config (git config --global, with include and includeIf followed). If git
// is unavailable, does not answer within 5 seconds, fails (a malformed global
// config, say), or the global config does not set it, the path is
// $XDG_CONFIG_HOME/git/ignore, or ~/.config/git/ignore when XDG_CONFIG_HOME
// is unset (none without a home directory). A global value set to the empty
// string loads nothing.
//
// This reads the global scope only; LoadRepo resolves core.excludesFile
// across every scope, as git does.
//
// If the resolved file does not exist, AddGlobalPatterns returns nil (no error).
// Only real read failures are returned as errors. A symlink is followed; a
// target that is not a regular file, or is larger than 100 MB, is an error
// wrapping ErrPatternFileSkipped.
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
	path, found, err := gitConfigGet("", "--global", "--includes", "--get", "core.excludesFile")
	if err != nil && !isExitError(err) {
		return fmt.Errorf("resolving global gitignore path: %w", err)
	}
	if !found {
		path = xdgGlobalIgnorePath()
	}
	return m.addExcludesFile(path, "global gitignore")
}

// AddSystemPatterns loads patterns from the system-scope gitignore file
// (referenced by `git config --system core.excludesFile`, typically
// configured via /etc/gitconfig, with include and includeIf followed) and
// adds them to the matcher.
//
// If git is unavailable, does not answer within 5 seconds, or fails (a
// malformed system config, say), the system config does not set
// core.excludesFile, or the referenced file does not exist, AddSystemPatterns
// returns nil (no error).
// Only real read failures are returned as errors. A symlink is followed; a
// target that is not a regular file, or is larger than 100 MB, is an error
// wrapping ErrPatternFileSkipped.
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
	path, _, err := gitConfigGet("", "--system", "--includes", "--get", "core.excludesFile")
	if err != nil && !isExitError(err) {
		return fmt.Errorf("resolving system gitignore path: %w", err)
	}
	return m.addExcludesFile(path, "system gitignore")
}

// addExcludesFile loads the pattern file at path with root scope, labeled
// with its path. An empty path or a missing file adds nothing; what names the
// file in a read error.
func (m *Matcher) addExcludesFile(path, what string) error {
	if path == "" {
		return nil
	}
	content, err := readPatternFile(osSource{}, path, true, maxPatternFileSize)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("reading %s %s: %w", what, path, err)
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
// with two extras: the file's path is recorded so that MatchResult.Source
// identifies it for any rule that originated here, and the file is checked
// before it is read, as described below.
//
// If path does not exist or cannot be read, the error is returned wrapped.
// A symlink is followed; a target that is not a regular file, or is larger
// than 100 MB, is an error wrapping ErrPatternFileSkipped. Empty files add no
// rules.
//
// Thread-safe: can be called concurrently with Match.
func (m *Matcher) AddPatternsFromFile(basePath, path string) error {
	content, err := readPatternFile(osSource{}, path, true, maxPatternFileSize)
	if err != nil {
		return fmt.Errorf("reading %s: %w", path, err)
	}
	m.addPatternsFromSource(basePath, content, path)
	return nil
}

// AddExcludePatterns loads patterns from the repository's .git/info/exclude
// file and adds them to the matcher. The gitDir parameter is the path to the
// .git directory (e.g., ".git" or an absolute path). It may also be a .git
// file, as in linked worktrees and submodules: its "gitdir:" pointer is
// followed, and then the git directory's commondir if present, so a linked
// worktree reads the main repository's info/exclude like git does. A .git
// file without a "gitdir:" line has no exclude file.
//
// If the exclude file does not exist, AddExcludePatterns returns nil (no error).
// Only real read failures are returned as errors. A symlink is followed, as
// in git; a target that is not a regular file, or is larger than 100 MB, is
// an error wrapping ErrPatternFileSkipped.
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
	dir, ok, err := resolveExcludeDir(gitDir)
	if err != nil || !ok {
		return err
	}
	return m.addExcludesFile(filepath.Join(dir, "info", "exclude"), "exclude file")
}

// resolveExcludeDir returns the directory whose info/exclude applies to the
// git path gitPath (a .git directory or a .git file). ok is false when gitPath
// does not exist or is a .git file without a gitdir pointer.
func resolveExcludeDir(gitPath string) (dir string, ok bool, err error) {
	fi, err := os.Stat(gitPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", false, nil
		}
		return "", false, fmt.Errorf("reading %s: %w", gitPath, err)
	}

	gitDir := gitPath
	if !fi.IsDir() {
		content, readErr := readPatternFile(osSource{}, gitPath, true, maxPatternFileSize)
		if readErr != nil {
			return "", false, fmt.Errorf("reading %s: %w", gitPath, readErr)
		}
		target, found := strings.CutPrefix(strings.TrimSpace(string(content)), "gitdir:")
		if !found {
			return "", false, nil
		}
		gitDir = resolveRelative(filepath.Dir(gitPath), strings.TrimSpace(target))
	}

	common, err := readPatternFile(osSource{}, filepath.Join(gitDir, "commondir"), true, maxPatternFileSize)
	switch {
	case err == nil:
		return resolveRelative(gitDir, strings.TrimSpace(string(common))), true, nil
	case errors.Is(err, os.ErrNotExist):
		return gitDir, true, nil
	default:
		return "", false, fmt.Errorf("reading %s: %w", filepath.Join(gitDir, "commondir"), err)
	}
}

// resolveRelative resolves p against base unless p is already absolute.
func resolveRelative(base, p string) string {
	p = filepath.FromSlash(p)
	if filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(base, p)
}

// effectiveExcludesFile returns the core.excludesFile git uses for repoRoot:
// the last value across every config scope, includes followed, or the XDG
// default when no scope sets it. "" means a value set to the empty string,
// which loads nothing.
func effectiveExcludesFile(repoRoot string) (string, error) {
	// git cannot run in a directory that does not exist, and LoadRepo
	// tolerates a missing repoRoot, so ask from its nearest existing ancestor.
	dir := repoRoot
	for {
		if _, err := os.Stat(dir); err == nil {
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	path, found, err := gitConfigGet(dir, "--get", "core.excludesFile")
	if isExitError(err) {
		// git refuses to run in a repository whose .git it rejects (a
		// malformed .git file, a gitdir pointer to a non-repository). Ask
		// again without the repository, so its config alone is skipped; a
		// malformed system or global config still fails here.
		path, found, err = gitConfigGetNoRepo("--get", "core.excludesFile")
	}
	if err != nil {
		return "", fmt.Errorf("resolving core.excludesFile: %w", err)
	}
	if found {
		return path, nil
	}
	return xdgGlobalIgnorePath(), nil
}

// gitConfigGet runs `git config --path <args...>` in dir ("" for the current
// directory). --path makes git expand ~ and ~user. found is false when the
// key is unset (exit 1), git is not installed, or the call timed out: each
// means "use the default". Any other exit is an error wrapping
// *exec.ExitError, with git's message.
func gitConfigGet(dir string, args ...string) (value string, found bool, err error) {
	// dir, not an inherited GIT_DIR or GIT_WORK_TREE, decides which
	// repository's config is read.
	return runGitConfig(dir, withoutEnv(os.Environ(), "GIT_DIR", "GIT_WORK_TREE"), args)
}

// gitConfigGetNoRepo is gitConfigGet outside any repository: git config
// treats a GIT_DIR that is not a git directory as no repository, and reads
// every other scope.
func gitConfigGetNoRepo(args ...string) (value string, found bool, err error) {
	env := append(withoutEnv(os.Environ(), "GIT_DIR", "GIT_WORK_TREE"), "GIT_DIR="+os.DevNull)
	return runGitConfig("", env, args)
}

// runGitConfig runs git config in dir with env; see gitConfigGet for results.
func runGitConfig(dir string, env, args []string) (value string, found bool, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), gitConfigTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", append([]string{"config", "--path"}, args...)...)
	cmd.Dir = dir
	cmd.Env = env
	// Killing git does not close stdout if a child of it still holds the pipe;
	// without WaitDelay, Output would block until that child exits.
	cmd.WaitDelay = time.Second
	out, err := cmd.Output()
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) || errors.Is(err, exec.ErrNotFound) {
			return "", false, nil
		}
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			return "", false, fmt.Errorf("running git config: %w", err)
		}
		if exitErr.ExitCode() == 1 {
			return "", false, nil
		}
		msg, _, _ := strings.Cut(strings.TrimSpace(string(exitErr.Stderr)), "\n")
		return "", false, fmt.Errorf("git config: %w: %s", err, msg)
	}
	return strings.TrimRight(string(out), "\r\n"), true, nil
}

// isExitError reports whether err comes from git exiting non-zero. The
// per-scope loaders treat that as "not set": a scoped git config fails on
// some platforms when the scope's file is missing.
func isExitError(err error) bool {
	var exitErr *exec.ExitError
	return errors.As(err, &exitErr)
}

// withoutEnv returns env without the named variables.
func withoutEnv(env []string, names ...string) []string {
	out := make([]string, 0, len(env))
	for _, kv := range env {
		name, _, _ := strings.Cut(kv, "=")
		// Windows environment names are case-insensitive.
		if !slices.ContainsFunc(names, func(n string) bool { return strings.EqualFold(n, name) }) {
			out = append(out, kv)
		}
	}
	return out
}

// xdgGlobalIgnorePath returns the XDG-based global gitignore path.
// Uses $XDG_CONFIG_HOME/git/ignore if set, otherwise ~/.config/git/ignore.
// With no home directory it returns "", as git then reads no default file.
func xdgGlobalIgnorePath() string {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "git", "ignore")
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "git", "ignore")
}
