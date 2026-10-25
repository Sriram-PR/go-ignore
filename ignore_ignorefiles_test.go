package ignore_test

import (
	"errors"
	"io/fs"
	"maps"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	ignore "github.com/Sriram-PR/go-ignore"
	"github.com/Sriram-PR/go-ignore/internal/paritycases"
)

// splitIgnoreFiles moves the second half of each .gitignore of s, cut after a
// line break, into a .aiignore in the same directory. A tail that would start
// with a BOM stays in .gitignore: at the start of a file the BOM is dropped,
// mid-file it is pattern text. ok is false when s already uses the name.
func splitIgnoreFiles(s paritycases.Scenario) (split paritycases.Scenario, ok bool) {
	files := map[string]string{}
	for f, content := range s.Files {
		ai := path.Join(path.Dir(f), ".aiignore")
		if _, taken := s.Paths[ai]; taken {
			return s, false
		}
		lines := strings.SplitAfter(content, "\n")
		k := len(lines) / 2
		head, tail := strings.Join(lines[:k], ""), strings.Join(lines[k:], "")
		if strings.HasPrefix(tail, "\ufeff") {
			head, tail = content, ""
		}
		files[f] = head
		files[ai] = tail
	}
	return paritycases.Scenario{Name: s.Name, ICase: s.ICase, Files: files, Paths: s.Paths}, true
}

// Splitting every .gitignore into .gitignore + .aiignore in the same
// directory changes nothing when IgnoreFiles names .aiignore: the walker and
// Tree read it right after .gitignore. The unsplit scenarios are what the
// parity harness compares with git.
func TestIgnoreFiles_SplitAgreesWithUnsplit(t *testing.T) {
	for _, s := range walkInvariantScenarios() {
		split, ok := splitIgnoreFiles(s)
		if !ok {
			continue
		}
		fsys, splitFS := scenarioFS(s), scenarioFS(split)
		opts := parityOptions(s)
		opts.IgnoreFiles = []string{".aiignore"}

		want := filesSet(t, ignore.NewWithOptions(parityOptions(s)).FilesFS(fsys, "."))
		got := filesSet(t, ignore.NewWithOptions(opts).FilesFS(splitFS, "."))
		for p := range got {
			if path.Base(p) == ".aiignore" {
				delete(got, p)
			}
		}
		if !maps.Equal(got, want) {
			t.Errorf("%s: FilesFS = %q, want %q\nsplit: %q", s.Name, slices.Sorted(maps.Keys(got)), slices.Sorted(maps.Keys(want)), split.Files)
		}

		wantTree := ignore.NewWithOptions(parityOptions(s)).TreeFS(fsys, ".")
		gotTree := ignore.NewWithOptions(opts).TreeFS(splitFS, ".")
		for p := range fsys {
			gr, gerr := gotTree.Check(p)
			wr, werr := wantTree.Check(p)
			if gerr != nil || werr != nil || gr.Ignored != wr.Ignored {
				t.Errorf("%s: Check(%q) = %v, %v; unsplit %v, %v\nsplit: %q", s.Name, p, gr.Ignored, gerr, wr.Ignored, werr, split.Files)
			}
		}
	}
}

// In one directory a later name beats an earlier one; a deeper .gitignore
// beats every file of a shallower directory.
func TestIgnoreFiles_Order(t *testing.T) {
	fsys := fstest.MapFS{
		".gitignore":   {Data: []byte("*.log\n")},
		".aiignore":    {Data: []byte("!keep.log\n")},
		".ignore":      {Data: []byte("keep.log\n")},
		"a/.gitignore": {Data: []byte("!keep.log\n")},
		"keep.log":     {},
		"x.log":        {},
		"a/keep.log":   {},
	}
	for _, c := range []struct {
		names   []string
		ignored bool   // keep.log
		source  string // the file that decides keep.log
	}{
		{[]string{".aiignore", ".ignore"}, true, ".ignore"},
		{[]string{".ignore", ".aiignore"}, false, ".aiignore"},
	} {
		m := ignore.NewWithOptions(ignore.MatcherOptions{IgnoreFiles: c.names})
		tr := m.TreeFS(fsys, ".")
		if r, err := tr.Check("keep.log"); err != nil || r.Ignored != c.ignored || r.Source != c.source || r.Line != 1 {
			t.Errorf("%q: Check(keep.log) = %+v, %v; want Ignored=%v from %s:1", c.names, r, err, c.ignored, c.source)
		}
		if r, err := tr.Check("a/keep.log"); err != nil || r.Ignored || r.Source != "a/.gitignore" {
			t.Errorf("%q: Check(a/keep.log) = %+v, %v; want re-included by a/.gitignore", c.names, r, err)
		}
		got := filesSet(t, m.FilesFS(fsys, "."))
		if got["keep.log"] == c.ignored || got["x.log"] || !got["a/keep.log"] {
			t.Errorf("%q: FilesFS = %v", c.names, slices.Sorted(maps.Keys(got)))
		}
	}
}

// Ignore files a directory can't read give one callback with all their
// errors; the files that could be read still apply.
func TestIgnoreFiles_RefusedFilesKeepTheOthers(t *testing.T) {
	fsys := fstest.MapFS{
		"sub/.gitignore": {Data: []byte("*.log\n")},
		"sub/.aiignore":  {Mode: fs.ModeNamedPipe},
		"sub/.ignore":    {Mode: fs.ModeNamedPipe},
		"sub/a.log":      {},
		"sub/b.txt":      {},
	}
	m := ignore.NewWithOptions(ignore.MatcherOptions{IgnoreFiles: []string{".aiignore", ".ignore"}})
	var subErrs []error
	var files []string
	err := m.WalkDirFS(fsys, ".", func(p string, d fs.DirEntry, walkErr error) error {
		if p == "sub" {
			subErrs = append(subErrs, walkErr)
		}
		if walkErr == nil && !d.IsDir() {
			files = append(files, p)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(subErrs) != 1 || !errors.Is(subErrs[0], ignore.ErrPatternFileSkipped) ||
		!strings.Contains(subErrs[0].Error(), "sub/.aiignore") || !strings.Contains(subErrs[0].Error(), "sub/.ignore") {
		t.Errorf("sub callbacks = %v, want one error naming both files", subErrs)
	}
	if want := []string{"sub/.aiignore", "sub/.gitignore", "sub/.ignore", "sub/b.txt"}; !slices.Equal(files, want) {
		t.Errorf("files = %q, want %q", files, want)
	}
	if r, err := m.TreeFS(fsys, ".").Check("sub/a.log"); !r.Ignored || !errors.Is(err, ignore.ErrPatternFileSkipped) {
		t.Errorf("Check(sub/a.log) = %+v, %v; want ignored, with the skipped files' error", r, err)
	}
}

func TestIgnoreFiles_BadNamesDropped(t *testing.T) {
	names := []string{"", ".", "..", ".gitignore", "a/b", `a\b`, ".x", ".x"}
	m := ignore.NewWithOptions(ignore.MatcherOptions{IgnoreFiles: names})
	names[6] = ".y"
	if w := m.Warnings(); len(w) != 7 {
		t.Errorf("Warnings() = %+v, want 7 (6 bad names, 1 repeat)", w)
	}
	fsys := fstest.MapFS{".x": {Data: []byte("f\n")}, ".y": {Data: []byte("g\n")}, "f": {}, "g": {}}
	if got := filesSet(t, m.FilesFS(fsys, ".")); got["f"] || !got["g"] {
		t.Errorf("FilesFS = %v: .x must apply (names copied at construction), .y must not", got)
	}
}

func TestLoadRepo_ReadsRootIgnoreFiles(t *testing.T) {
	dir := t.TempDir()
	for name, content := range map[string]string{".gitignore": "*.log\n", ".aiignore": "!keep.log\n"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	m, err := ignore.LoadRepo(dir, ignore.MatcherOptions{IgnoreFiles: []string{".aiignore"}})
	if err != nil {
		t.Fatal(err)
	}
	if !m.Match("x.log", false) {
		t.Error("x.log must be ignored by the root .gitignore")
	}
	if r := m.MatchWithReason("keep.log", false); r.Ignored || r.Source != filepath.Join(dir, ".aiignore") {
		t.Errorf("keep.log: %+v, want re-included by the root .aiignore", r)
	}
}
