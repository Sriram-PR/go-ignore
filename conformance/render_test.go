package conformance

import (
	"strings"
	"testing"
)

func sampleResults() *results {
	return &results{
		Drivers: []driverInfo{{"go-ignore", "(this repo)", "A"}, {"lib-b", "v1.0.0", "B"}},
		Rows:    []string{"Whitespace & escapes", "Nested .gitignore files", randomRow, totalRow},
		Cells: map[string][]cell{
			"Whitespace & escapes":    {{Agree: 2, Total: 2}, {Agree: 1, Total: 2}},
			"Nested .gitignore files": {{Agree: 3, Total: 3}, {Skipped: 2}},
			randomRow:                 {{Agree: 5, Total: 5}, {Agree: 3, Total: 4, Skipped: 1}},
			totalRow:                  {{Agree: 10, Total: 10}, {Agree: 4, Total: 6, Skipped: 3}},
		},
		Disagree: [][]disagreement{nil, nil},
	}
}

func TestRenderTable(t *testing.T) {
	want := `| Feature | go-ignore | lib-b |
|---|--:|--:|
| Whitespace & escapes | 2/2 | 1/2 |
| Nested .gitignore files | 3/3 | n/a |
| Random scenarios (seed 1) | 5/5 | 3/4 † |
| **Total** | 10/10 | 4/6 † |

† excludes scenarios with nested .gitignore files, which this library can't load.
`
	if got := renderTable(sampleResults()); got != want {
		t.Errorf("renderTable:\n%s\nwant:\n%s", got, want)
	}
}

func TestRenderTable_NoNoteWithoutSkips(t *testing.T) {
	r := sampleResults()
	r.Rows = []string{"Whitespace & escapes"}
	if got := renderTable(r); strings.Contains(got, "†") {
		t.Errorf("renderTable without skipped cells has a † note:\n%s", got)
	}
}

func TestDisagreementRow(t *testing.T) {
	d := disagreement{Scenario: "curated-3", Files: map[string]string{".gitignore": "a|*b\n"}, Path: "x_y", Git: true}
	want := `| curated-3 | {".gitignore":"a\|\*b\\n"} | "x\_y" | false | true | false |`
	if got := disagreementRow(d); got != want {
		t.Errorf("disagreementRow = %s\nwant %s", got, want)
	}
	d.Err = "panic: x\ny"
	if got := disagreementRow(d); !strings.HasSuffix(got, "| error: panic: x y |") {
		t.Errorf("disagreementRow with Err = %s", got)
	}
}

func TestRenderResults(t *testing.T) {
	r := sampleResults()
	r.Disagree[1] = []disagreement{{Scenario: "seed1-4", Files: map[string]string{".gitignore": "a\n"}, Path: "a", Git: true}}
	got := renderResults(r, "git version 2.56.0")
	for _, want := range []string{
		"\nGit version: git version 2.56.0\n",
		"| lib-b | v1.0.0 | B |\n",
		"| **Total** | 10/10 | 4/6 † |\n",
		"### go-ignore\n\nNone.\n",
		"### lib-b\n\n| Scenario | Files | Path | Dir | git | lib |\n|---|---|---|---|---|---|\n| seed1-4 |",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("renderResults lacks %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "—") {
		t.Error("renderResults contains an em dash")
	}
}

func TestStripGitVersion(t *testing.T) {
	a := "x\nGit version: git version 2.56.0\ny\n"
	b := "x\nGit version: git version 2.43.0\ny\n"
	if stripGitVersion(a) != stripGitVersion(b) {
		t.Error("stripGitVersion leaves the version in")
	}
	if stripGitVersion("x\ny\n") == stripGitVersion("x\nz\n") {
		t.Error("stripGitVersion drops other lines")
	}
}

func TestSpliceReadme(t *testing.T) {
	in := "before\n" + readmeStart + "\nold\n" + readmeEnd + "\nafter\n"
	got, err := spliceReadme(in, "| t |\n")
	if err != nil {
		t.Fatal(err)
	}
	if want := "before\n" + readmeStart + "\n| t |\n" + readmeEnd + "\nafter\n"; got != want {
		t.Errorf("spliceReadme = %q, want %q", got, want)
	}
	for name, bad := range map[string]string{
		"missing":  "no markers\n",
		"twice":    readmeStart + readmeEnd + readmeStart + readmeEnd,
		"reversed": readmeEnd + "\n" + readmeStart + "\n",
	} {
		if _, err := spliceReadme(bad, "x\n"); err == nil {
			t.Errorf("%s: spliceReadme returned no error", name)
		}
	}
}
