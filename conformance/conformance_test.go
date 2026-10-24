package conformance

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

const parityBaseline = "../testdata/parity/baseline.txt"

const (
	resultsFile = "RESULTS.md"
	readmeFile  = "../README.md"
)

// requireGit skips without git, unless CONFORMANCE_REQUIRE_GIT=1 (set in CI),
// which turns the skip into a failure.
func requireGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		if os.Getenv("CONFORMANCE_REQUIRE_GIT") == "1" {
			t.Fatal("CONFORMANCE_REQUIRE_GIT=1 but git is not on PATH")
		}
		t.Skip("git is not on PATH")
	}
}

// TestConformance runs every scenario through git and each library, and
// fails when RESULTS.md or the README table differ from what the run
// renders. CONFORMANCE_UPDATE=1 rewrites them instead.
func TestConformance(t *testing.T) {
	requireGit(t)
	res, err := run(scenarios(), drivers, t.TempDir)
	if err != nil {
		t.Fatal(err)
	}
	checkParityBaseline(t, res)

	out, err := exec.Command("git", "--version").Output()
	if err != nil {
		t.Fatal(err)
	}
	resultsMD := renderResults(res, strings.TrimSpace(string(out)))
	readme, err := os.ReadFile(readmeFile)
	if err != nil {
		t.Fatal(err)
	}
	newReadme, err := spliceReadme(string(readme), renderTable(res))
	if err != nil {
		t.Fatal(err)
	}

	if os.Getenv("CONFORMANCE_UPDATE") == "1" {
		if writeErr := os.WriteFile(resultsFile, []byte(resultsMD), 0o644); writeErr != nil {
			t.Fatal(writeErr)
		}
		if writeErr := os.WriteFile(readmeFile, []byte(newReadme), 0o644); writeErr != nil {
			t.Fatal(writeErr)
		}
		return
	}
	committed, err := os.ReadFile(resultsFile)
	if err != nil {
		t.Fatalf("%v; run make conformance-update", err)
	}
	if stripGitVersion(string(committed)) != stripGitVersion(resultsMD) {
		t.Error("conformance/RESULTS.md is stale; run make conformance-update and commit it")
	}
	if string(readme) != newReadme {
		t.Error("the README conformance table is stale; run make conformance-update and commit it")
	}
}

// checkParityBaseline fails on any go-ignore disagreement the parity harness
// does not already list, so the two harnesses cannot drift apart.
func checkParityBaseline(t *testing.T, res *results) {
	t.Helper()
	if res.Drivers[0].Name != "go-ignore" {
		t.Fatalf("drivers[0] is %q, want go-ignore", res.Drivers[0].Name)
	}
	data, err := os.ReadFile(parityBaseline)
	if err != nil {
		t.Fatal(err)
	}
	known := map[string]bool{}
	for line := range strings.SplitSeq(string(data), "\n") {
		if line != "" && !strings.HasPrefix(line, "#") {
			known[line] = true
		}
	}
	for _, d := range res.Disagree[0] {
		if line := baselineLine(d); d.Err != "" || !known[line] {
			t.Errorf("go-ignore disagrees with git outside the parity baseline: %s (%s, err %q)", line, d.Scenario, d.Err)
		}
	}
}
