package conformance

import (
	"errors"
	"strings"
	"testing"

	"github.com/Sriram-PR/go-ignore/internal/paritycases"
)

// Every driver agrees with git on a case all of them handle, so a broken
// adapter fails here rather than as a bad score.
func TestDrivers_Smoke(t *testing.T) {
	requireGit(t)
	s := paritycases.Scenario{
		Name:  "smoke",
		Files: map[string]string{".gitignore": "*.log\n"},
		Paths: map[string]bool{"a.log": false, "b.txt": false},
	}
	res, err := run([]paritycases.Scenario{s}, drivers, t.TempDir)
	if err != nil {
		t.Fatal(err)
	}
	for i, d := range res.Drivers {
		if len(res.Disagree[i]) > 0 {
			t.Errorf("%s disagrees with git: %+v", d.Name, res.Disagree[i])
		}
		if c := res.Cells[totalRow][i]; c.Agree != 2 || c.Total != 2 || c.Skipped != 0 {
			t.Errorf("%s: total cell %+v, want 2/2 with nothing skipped", d.Name, c)
		}
		if d.Version == "unknown" {
			t.Errorf("%s: version unknown", d.Name)
		}
	}
}

func TestRun_SkipsNestedAndRecordsFailures(t *testing.T) {
	requireGit(t)
	fake := []driver{
		{name: "flat", load: func(string) (matchFunc, error) {
			return func(string, bool) bool { return true }, nil
		}},
		{name: "broken", nested: true, load: func(string) (matchFunc, error) {
			return nil, errors.New("boom")
		}},
		{name: "panics", nested: true, load: func(string) (matchFunc, error) {
			return func(string, bool) bool { panic("bad") }, nil
		}},
	}
	s := paritycases.Scenario{
		Name:  "nested",
		Files: map[string]string{".gitignore": "*.log\n", "sub/.gitignore": "x\n"},
		Paths: map[string]bool{"a.log": false, "sub": true, "sub/x": false},
	}
	res, err := run([]paritycases.Scenario{s}, fake, t.TempDir)
	if err != nil {
		t.Fatal(err)
	}
	if c := res.Cells[randomRow][0]; c != (cell{Skipped: 1}) {
		t.Errorf("flat: random cell %+v, want only Skipped=1", c)
	}
	if len(res.Disagree[0]) != 0 {
		t.Errorf("flat: %d disagreements, want 0 (skipped)", len(res.Disagree[0]))
	}
	for i, want := range map[int]string{1: "boom", 2: "panic: bad"} {
		if c := res.Cells[totalRow][i]; c.Agree != 0 || c.Total != 3 {
			t.Errorf("%s: total cell %+v, want 0/3", res.Drivers[i].Name, c)
		}
		if len(res.Disagree[i]) != 3 {
			t.Fatalf("%s: %d disagreements, want 3", res.Drivers[i].Name, len(res.Disagree[i]))
		}
		for _, d := range res.Disagree[i] {
			if !strings.Contains(d.Err, want) {
				t.Errorf("%s: %s err = %q, want it to contain %q", res.Drivers[i].Name, d.Path, d.Err, want)
			}
		}
	}
}

func TestBaselineLine(t *testing.T) {
	d := disagreement{
		Files: map[string]string{"b/.gitignore": "c\n", ".gitignore": "a\n"},
		Path:  "x y",
		IsDir: true,
		Git:   true,
	}
	want := `match files={".gitignore":"a\n" "b/.gitignore":"c\n"} path="x y" dir=true git=true lib=false`
	if got := baselineLine(d); got != want {
		t.Errorf("baselineLine = %s\nwant %s", got, want)
	}
}
