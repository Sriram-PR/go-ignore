package ignore_test

import (
	"os"
	"os/exec"
	"testing"

	"github.com/Sriram-PR/go-ignore/internal/paritycases"
)

// requireParity skips when GOIGNORE_PARITY is unset, and fails when it is set
// to anything but 1 or git is missing, so CI cannot pass without running the
// comparison.
func requireParity(t *testing.T) {
	t.Helper()
	switch v := os.Getenv("GOIGNORE_PARITY"); v {
	case "":
		t.Skip("set GOIGNORE_PARITY=1 to run the git parity harness")
	case "1":
	default:
		t.Fatalf("GOIGNORE_PARITY must be 1, got %q", v)
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Fatal("GOIGNORE_PARITY=1 but git is not on PATH")
	}
}

// gitRepo writes s into a fresh git repository and returns its directory and
// each path's type as found on disk.
func gitRepo(t *testing.T, s paritycases.Scenario) (string, map[string]bool) {
	t.Helper()
	dir := t.TempDir()
	if err := paritycases.InitRepo(dir, s); err != nil {
		t.Fatal(err)
	}
	isDir, err := paritycases.Materialize(dir, s)
	if err != nil {
		t.Fatal(err)
	}
	return dir, isDir
}

// gitCheckIgnore reports, for every path, whether git ignores it.
func gitCheckIgnore(t *testing.T, s paritycases.Scenario, dir string, paths []string) map[string]bool {
	t.Helper()
	res := make(map[string]bool, len(paths))
	for p, v := range gitCheckIgnoreVerbose(t, s, dir, paths) {
		res[p] = v.Ignored
	}
	return res
}

func gitCheckIgnoreVerbose(t *testing.T, s paritycases.Scenario, dir string, paths []string) map[string]paritycases.Verdict {
	t.Helper()
	v, err := paritycases.GitVerdicts(dir, s, paths)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func gitLsFiles(t *testing.T, s paritycases.Scenario, dir string) map[string]bool {
	t.Helper()
	files, err := paritycases.GitLsFiles(dir, s)
	if err != nil {
		t.Fatal(err)
	}
	return files
}
