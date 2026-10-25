package ignore_test

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	ignore "github.com/Sriram-PR/go-ignore"
)

// Walking from any directory yields exactly the full walk's entries under
// it: the rules of root and every directory above Start are loaded first,
// and a Start the full walk never reaches yields nothing.
func TestEntries_StartAgreesWithFullWalk(t *testing.T) {
	for _, s := range walkInvariantScenarios() {
		fsys := scenarioFS(s)
		m := ignore.NewWithOptions(parityOptions(s))
		full := entryVisits(t, m.EntriesFS(fsys, ".", ignore.WalkOptions{}), nil)
		for d, f := range fsys {
			// A backslash is a valid name byte on Unix but not in Start.
			if !f.Mode.IsDir() || strings.Contains(d, `\`) {
				continue
			}
			var want []visit
			if slices.Contains(full, visit{d, true}) {
				for _, v := range full {
					if strings.HasPrefix(v.path, d+"/") {
						want = append(want, v)
					}
				}
			}
			got := entryVisits(t, m.EntriesFS(fsys, ".", ignore.WalkOptions{Start: d}), nil)
			if !slices.Equal(got, want) {
				t.Errorf("%s: Start %q = %v, want %v\nfiles: %q", s.Name, d, got, want, s.Files)
			}
		}
	}
}

type startYield struct {
	path string
	err  error
}

func startYields(fsys fs.FS, start string) []startYield {
	var got []startYield
	for e, err := range ignore.New().EntriesFS(fsys, ".", ignore.WalkOptions{Start: start}) {
		got = append(got, startYield{e.Path, err})
	}
	return got
}

func TestEntries_StartCases(t *testing.T) {
	fsys := fstest.MapFS{
		".gitignore":     {Data: []byte("build/\n*.log\n!keep.log\n")},
		"src/.gitignore": {Data: []byte("gen/\n")},
		"src/a.go":       {},
		"src/a.log":      {},
		"src/keep.log":   {},
		"src/gen/x.go":   {},
		"src/sub/b.go":   {},
		"build/out":      {},
		"link":           {Mode: fs.ModeSymlink, Data: []byte("src")},
		"file.txt":       {},
	}
	paths := map[string][]string{
		"src":       {"src/.gitignore", "src/a.go", "src/keep.log", "src/sub", "src/sub/b.go"},
		"src/sub":   {"src/sub/b.go"},
		"src/a.go":  {"src/a.go"},
		"src/a.log": nil, // ignored by the root .gitignore
		"src/gen":   nil, // ignored by src/.gitignore
		"build":     nil,
		"build/out": nil, // below an ignored directory
	}
	for start, want := range paths {
		var got []string
		for _, y := range startYields(fsys, start) {
			if y.err != nil {
				t.Errorf("Start %q: %s: %v", start, y.path, y.err)
				continue
			}
			got = append(got, y.path)
		}
		if !slices.Equal(got, want) {
			t.Errorf("Start %q = %q, want %q", start, got, want)
		}
	}

	errs := map[string]string{ // Start -> Entry.Path of its one error
		"missing":    "missing",
		"link":       "link",
		"link/a.go":  "link",
		"file.txt/x": "file.txt",
		"../x":       "",
		"/src":       "",
		"src//sub":   "",
		"./src":      "",
		`src\sub`:    "",
		"src/":       "",
	}
	for start, wantPath := range errs {
		got := startYields(fsys, start)
		if len(got) != 1 || got[0].err == nil || got[0].path != wantPath {
			t.Errorf("Start %q = %v, want one error at %q", start, got, wantPath)
		}
	}
	if got := startYields(fsys, "missing"); len(got) == 1 && !errors.Is(got[0].err, fs.ErrNotExist) {
		t.Errorf("Start missing: err = %v, want fs.ErrNotExist", got[0].err)
	}
}

// An unreadable ignore file above Start is reported with that directory;
// continuing walks on without it, SkipDir ends the walk.
func TestEntries_StartPreloadError(t *testing.T) {
	fsys := fstest.MapFS{
		"a/.gitignore": {Mode: fs.ModeNamedPipe},
		"a/b/c.txt":    {},
	}
	for _, skip := range []bool{false, true} {
		var got []string
		for e, err := range ignore.New().EntriesFS(fsys, ".", ignore.WalkOptions{Start: "a/b"}) {
			if err != nil {
				if e.Path != "a" || !errors.Is(err, ignore.ErrPatternFileSkipped) {
					t.Errorf("skip=%v: error %q, %v; want a, ErrPatternFileSkipped", skip, e.Path, err)
				}
				if skip {
					e.SkipDir()
				}
				continue
			}
			got = append(got, e.Path)
		}
		var want []string
		if !skip {
			want = []string{"a/b/c.txt"}
		}
		if !slices.Equal(got, want) {
			t.Errorf("skip=%v: got %q, want %q", skip, got, want)
		}
	}
}

func TestEntries_StartOS(t *testing.T) {
	root := t.TempDir()
	for name, content := range map[string]string{".gitignore": "*.log\n", "src/a.go": "", "src/b.log": "", "other/c.go": ""} {
		p := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	var got []string
	for e, err := range ignore.New().Entries(root, ignore.WalkOptions{Start: "src"}) {
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, e.Path)
	}
	if want := []string{filepath.Join(root, "src", "a.go")}; !slices.Equal(got, want) {
		t.Errorf("Entries from src = %q, want %q", got, want)
	}
}
