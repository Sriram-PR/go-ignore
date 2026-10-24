package conformance

import (
	"fmt"
	"maps"
	"path"
	"slices"
	"strings"

	"github.com/Sriram-PR/go-ignore/internal/paritycases"
)

const (
	randomRow = "Random scenarios (seed 1)"
	totalRow  = "Total"
)

// cell counts one library's answers in one table row.
type cell struct {
	Agree   int // paths answered as git does
	Total   int // paths compared
	Skipped int // scenarios left out: nested files the library can't load
}

// disagreement is one path where a library and git differ. When Err is set
// (load error or panic) Lib is meaningless.
type disagreement struct {
	Scenario string
	Files    map[string]string
	Path     string
	IsDir    bool
	Git      bool
	Lib      bool
	Err      string
}

type driverInfo struct {
	Name, Version, API string
}

// results is everything the renderer needs. Each Cells row and Disagree are
// indexed like Drivers.
type results struct {
	Drivers  []driverInfo
	Rows     []string
	Cells    map[string][]cell
	Disagree [][]disagreement
}

// scenarios is the conformance set: the curated cases without the
// case-insensitive ones, which no other library supports, then 300 seeded
// scenarios from seed 1.
func scenarios() []paritycases.Scenario {
	var out []paritycases.Scenario
	for _, s := range paritycases.Curated() {
		if !s.ICase {
			out = append(out, s)
		}
	}
	return append(out, paritycases.Seeded(1, 300, false)...)
}

// materialized is one scenario on disk with git's answers.
type materialized struct {
	s        paritycases.Scenario
	row      string
	dir      string
	isDir    map[string]bool
	paths    []string
	verdicts map[string]paritycases.Verdict
}

// run writes each scenario into a fresh directory from newDir, asks git, and
// compares every driver's answers. A path git does not report is an error:
// the table must not count over a hole.
func run(scens []paritycases.Scenario, ds []driver, newDir func() string) (*results, error) {
	r := &results{
		Rows:     append(slices.Clone(paritycases.Groups), randomRow, totalRow),
		Cells:    map[string][]cell{},
		Disagree: make([][]disagreement, len(ds)),
	}
	for _, d := range ds {
		r.Drivers = append(r.Drivers, driverInfo{d.name, moduleVersion(d.module), d.api})
	}
	for _, row := range r.Rows {
		r.Cells[row] = make([]cell, len(ds))
	}
	for _, s := range scens {
		m, err := materialize(s, newDir())
		if err != nil {
			return nil, err
		}
		for i, d := range ds {
			r.compare(i, d, m)
		}
	}
	return r, nil
}

func materialize(s paritycases.Scenario, dir string) (materialized, error) {
	m := materialized{s: s, row: s.Group, dir: dir}
	if m.row == "" {
		m.row = randomRow
	}
	if err := paritycases.InitRepo(dir, s); err != nil {
		return m, err
	}
	isDir, err := paritycases.Materialize(dir, s)
	if err != nil {
		return m, err
	}
	m.isDir = isDir
	for _, p := range slices.Sorted(maps.Keys(isDir)) {
		if path.Base(p) != ".gitignore" {
			m.paths = append(m.paths, p)
		}
	}
	if m.verdicts, err = paritycases.GitVerdicts(dir, s, m.paths); err != nil {
		return m, err
	}
	for _, p := range m.paths {
		if _, ok := m.verdicts[p]; !ok {
			return m, fmt.Errorf("%s: git did not report %q", s.Name, p)
		}
	}
	return m, nil
}

func (r *results) compare(i int, d driver, m materialized) {
	if !d.nested && hasNested(m.s) {
		r.Cells[m.row][i].Skipped++
		r.Cells[totalRow][i].Skipped++
		return
	}
	match, loadErr := safeLoad(d, m.dir)
	for _, p := range m.paths {
		git := m.verdicts[p].Ignored
		lib, errText := false, ""
		if loadErr != nil {
			errText = loadErr.Error()
		} else if v, matchErr := safeMatch(match, p, m.isDir[p]); matchErr != nil {
			errText = matchErr.Error()
		} else {
			lib = v
		}
		r.Cells[m.row][i].Total++
		r.Cells[totalRow][i].Total++
		if errText == "" && lib == git {
			r.Cells[m.row][i].Agree++
			r.Cells[totalRow][i].Agree++
			continue
		}
		r.Disagree[i] = append(r.Disagree[i], disagreement{m.s.Name, m.s.Files, p, m.isDir[p], git, lib, errText})
	}
}

func hasNested(s paritycases.Scenario) bool {
	for f := range s.Files {
		if f != ".gitignore" {
			return true
		}
	}
	return false
}

// safeLoad and safeMatch turn a library panic into an error, so it shows in
// the table as a disagreement instead of stopping the run.
func safeLoad(d driver, dir string) (m matchFunc, err error) {
	defer func() {
		if p := recover(); p != nil {
			err = fmt.Errorf("panic: %v", p)
		}
	}()
	return d.load(dir)
}

func safeMatch(m matchFunc, p string, isDir bool) (ignored bool, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic: %v", r)
		}
	}()
	return m(p, isDir), nil
}

// formatFiles renders a scenario's files the way a parity baseline record
// does (parityRecord.String in the library's parity_test.go).
func formatFiles(files map[string]string) string {
	var b strings.Builder
	b.WriteString("{")
	for i, name := range slices.Sorted(maps.Keys(files)) {
		if i > 0 {
			b.WriteString(" ")
		}
		fmt.Fprintf(&b, "%q:%q", name, files[name])
	}
	b.WriteString("}")
	return b.String()
}

// baselineLine is d as a match-mode line of testdata/parity/baseline.txt.
func baselineLine(d disagreement) string {
	return fmt.Sprintf("match files=%s path=%q dir=%v git=%v lib=%v", formatFiles(d.Files), d.Path, d.IsDir, d.Git, d.Lib)
}
