package paritycases

import (
	"bytes"
	"errors"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

// Verdict is git check-ignore -v's answer for one path. Source and Line are
// empty and 0 when no pattern matched.
type Verdict struct {
	Source  string
	Line    int
	Ignored bool
}

// gitEnv points every place git reads user or system config and global
// excludes from at a missing path, so the developer's setup never leaks in.
func gitEnv() []string {
	missing := filepath.Join(os.TempDir(), "paritycases-no-such-dir")
	return append(os.Environ(),
		"HOME="+missing,
		"USERPROFILE="+missing,
		"XDG_CONFIG_HOME="+missing,
		"GIT_CONFIG_GLOBAL="+missing,
		"GIT_CONFIG_SYSTEM="+missing,
	)
}

// runGit runs git in dir. Exit code 1 with empty stderr is not an error:
// check-ignore uses it for "nothing ignored".
func runGit(s Scenario, dir string, stdin []byte, args ...string) ([]byte, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = gitEnv()
	if stdin != nil {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	var out, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &stderr
	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) || exitErr.ExitCode() != 1 || stderr.Len() > 0 {
			return nil, fmt.Errorf("%s: git %v: %w\nstderr: %s\nfiles: %q", s.Name, args, err, stderr.String(), s.Files)
		}
	}
	return out.Bytes(), nil
}

// InitRepo makes dir a git repository for s.
func InitRepo(dir string, s Scenario) error {
	if _, err := runGit(s, dir, nil, "init", "-q"); err != nil {
		return err
	}
	if s.ICase {
		if _, err := runGit(s, dir, nil, "config", "core.ignorecase", "true"); err != nil {
			return err
		}
	}
	return nil
}

// Materialize creates s's .gitignore files and paths under dir and returns
// whether each path is a directory on disk.
func Materialize(dir string, s Scenario) (map[string]bool, error) {
	for f, content := range s.Files {
		full := filepath.Join(dir, filepath.FromSlash(f))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			return nil, fmt.Errorf("%s: %w", s.Name, err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			return nil, fmt.Errorf("%s: %w", s.Name, err)
		}
	}
	for _, p := range slices.Sorted(maps.Keys(s.Paths)) {
		full := filepath.Join(dir, filepath.FromSlash(p))
		if s.Paths[p] {
			if err := os.MkdirAll(full, 0o755); err != nil {
				return nil, fmt.Errorf("%s: %w", s.Name, err)
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			return nil, fmt.Errorf("%s: %w", s.Name, err)
		}
		if _, err := os.Lstat(full); err == nil {
			continue
		}
		if err := os.WriteFile(full, nil, 0o644); err != nil {
			return nil, fmt.Errorf("%s: %w", s.Name, err)
		}
	}
	isDir := make(map[string]bool, len(s.Paths))
	for p := range s.Paths {
		fi, err := os.Lstat(filepath.Join(dir, filepath.FromSlash(p)))
		if err != nil {
			return nil, fmt.Errorf("%s: %w", s.Name, err)
		}
		isDir[p] = fi.IsDir()
	}
	return isDir, nil
}

// GitVerdicts asks git about every path in one check-ignore call. A path is
// ignored when git reports a matching pattern that is not a negation.
func GitVerdicts(dir string, s Scenario, paths []string) (map[string]Verdict, error) {
	var in bytes.Buffer
	for _, p := range paths {
		in.WriteString(p)
		in.WriteByte(0)
	}
	out, err := runGit(s, dir, in.Bytes(), "check-ignore", "--no-index", "-v", "-n", "-z", "--stdin")
	if err != nil {
		return nil, err
	}
	fields := strings.Split(string(out), "\x00")
	res := make(map[string]Verdict, len(paths))
	for i := 0; i+3 < len(fields); i += 4 {
		source, pattern, p := fields[i], fields[i+2], fields[i+3]
		line, _ := strconv.Atoi(fields[i+1])
		res[p] = Verdict{source, line, source != "" && !strings.HasPrefix(pattern, "!")}
	}
	return res, nil
}

// GitLsFiles returns the untracked, non-ignored files git lists.
func GitLsFiles(dir string, s Scenario) (map[string]bool, error) {
	out, err := runGit(s, dir, nil, "ls-files", "-o", "--exclude-standard", "-z")
	if err != nil {
		return nil, err
	}
	res := map[string]bool{}
	for p := range strings.SplitSeq(string(out), "\x00") {
		if p != "" {
			res[p] = true
		}
	}
	return res, nil
}
