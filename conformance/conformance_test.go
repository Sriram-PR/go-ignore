package conformance

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

const parityBaseline = "../testdata/parity/baseline.txt"

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

func TestConformance(t *testing.T) {
	requireGit(t)
	res, err := run(scenarios(), drivers, t.TempDir)
	if err != nil {
		t.Fatal(err)
	}
	checkParityBaseline(t, res)
	for i, d := range res.Drivers {
		c := res.Cells[totalRow][i]
		t.Logf("%s %s: %d/%d agree, %d scenarios skipped", d.Name, d.Version, c.Agree, c.Total, c.Skipped)
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
