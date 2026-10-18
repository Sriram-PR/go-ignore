package ignore_test

import (
	"bytes"
	"errors"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// requireParity skips unless GOIGNORE_PARITY=1, and fails if it is set but
// git is missing so CI cannot pass without running the comparison.
func requireParity(t *testing.T) {
	t.Helper()
	if os.Getenv("GOIGNORE_PARITY") != "1" {
		t.Skip("set GOIGNORE_PARITY=1 to run the git parity harness")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Fatal("GOIGNORE_PARITY=1 but git is not on PATH")
	}
}

// runGit runs git in dir and fails the test on any error, except exit code 1
// with empty stderr, which check-ignore uses for "nothing ignored".
func runGit(t *testing.T, s scenario, dir string, stdin []byte, args ...string) []byte {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if stdin != nil {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	var out, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &stderr
	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) || exitErr.ExitCode() != 1 || stderr.Len() > 0 {
			t.Fatalf("%s: git %v: %v\nstderr: %s\nfiles: %q", s.name, args, err, stderr.String(), s.files)
		}
	}
	return out.Bytes()
}

// gitRepo writes s into a fresh git repository and returns its directory and
// each path's type as found on disk.
func gitRepo(t *testing.T, s scenario) (string, map[string]bool) {
	t.Helper()
	dir := t.TempDir()
	runGit(t, s, dir, nil, "init", "-q")
	if s.icase {
		runGit(t, s, dir, nil, "config", "core.ignorecase", "true")
	}
	for f, content := range s.files {
		full := filepath.Join(dir, filepath.FromSlash(f))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("%s: %v", s.name, err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatalf("%s: %v", s.name, err)
		}
	}
	for _, p := range slices.Sorted(maps.Keys(s.paths)) {
		full := filepath.Join(dir, filepath.FromSlash(p))
		if s.paths[p] {
			if err := os.MkdirAll(full, 0o755); err != nil {
				t.Fatalf("%s: %v", s.name, err)
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("%s: %v", s.name, err)
		}
		if _, err := os.Lstat(full); err == nil {
			continue
		}
		if err := os.WriteFile(full, nil, 0o644); err != nil {
			t.Fatalf("%s: %v", s.name, err)
		}
	}
	isDir := make(map[string]bool, len(s.paths))
	for p := range s.paths {
		fi, err := os.Lstat(filepath.Join(dir, filepath.FromSlash(p)))
		if err != nil {
			t.Fatalf("%s: %v", s.name, err)
		}
		isDir[p] = fi.IsDir()
	}
	return dir, isDir
}

// gitCheckIgnore asks git about every path in one call. A path is ignored
// when git reports a matching pattern that is not a negation.
func gitCheckIgnore(t *testing.T, s scenario, dir string, paths []string) map[string]bool {
	t.Helper()
	var in bytes.Buffer
	for _, p := range paths {
		in.WriteString(p)
		in.WriteByte(0)
	}
	out := runGit(t, s, dir, in.Bytes(), "check-ignore", "--no-index", "-v", "-n", "-z", "--stdin")
	fields := strings.Split(string(out), "\x00")
	res := make(map[string]bool, len(paths))
	for i := 0; i+3 < len(fields); i += 4 {
		source, pattern, p := fields[i], fields[i+2], fields[i+3]
		res[p] = source != "" && !strings.HasPrefix(pattern, "!")
	}
	return res
}

// gitLsFiles returns the untracked, non-ignored files git lists.
func gitLsFiles(t *testing.T, s scenario, dir string) map[string]bool {
	t.Helper()
	out := runGit(t, s, dir, nil, "ls-files", "-o", "--exclude-standard", "-z")
	res := map[string]bool{}
	for p := range strings.SplitSeq(string(out), "\x00") {
		if p != "" {
			res[p] = true
		}
	}
	return res
}
