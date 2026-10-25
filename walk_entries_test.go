package ignore_test

import (
	"errors"
	"io/fs"
	"iter"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"testing/fstest"

	ignore "github.com/Sriram-PR/go-ignore"
)

type visit struct {
	path  string
	isDir bool
}

// walkVisits is what WalkDirFS passes fn, root left out, returning
// fs.SkipDir after each directory in skip.
func walkVisits(t *testing.T, m *ignore.Matcher, fsys fs.FS, skip map[string]bool) []visit {
	t.Helper()
	var got []visit
	err := m.WalkDirFS(fsys, ".", func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			t.Fatalf("WalkDirFS %s: %v", p, walkErr)
		}
		if p == "." {
			return nil
		}
		got = append(got, visit{p, d.IsDir()})
		if d.IsDir() && skip[p] {
			return fs.SkipDir
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return got
}

// entryVisits ranges over seq, calling SkipDir on each entry in skip.
func entryVisits(t *testing.T, seq iter.Seq2[ignore.Entry, error], skip map[string]bool) []visit {
	t.Helper()
	var got []visit
	for e, err := range seq {
		if err != nil {
			t.Fatalf("Entries %s: %v", e.Path, err)
		}
		got = append(got, visit{e.Path, e.IsDir()})
		if skip[e.Path] {
			e.SkipDir()
		}
	}
	return got
}

// Entries yields what WalkDirFS passes fn, without the root, and SkipDir
// prunes as returning fs.SkipDir from fn does.
func TestEntries_AgreesWithWalkDir(t *testing.T) {
	for _, s := range walkInvariantScenarios() {
		fsys := scenarioFS(s)
		skip := map[string]bool{}
		for p, f := range fsys {
			if f.Mode.IsDir() && len(p)%2 == 0 {
				skip[p] = true
			}
		}
		for _, sk := range []map[string]bool{nil, skip} {
			want := walkVisits(t, ignore.NewWithOptions(parityOptions(s)), fsys, sk)
			got := entryVisits(t, ignore.NewWithOptions(parityOptions(s)).EntriesFS(fsys, ".", ignore.WalkOptions{}), sk)
			if !slices.Equal(got, want) {
				t.Errorf("%s (skip %v): Entries = %v, want %v", s.Name, sk, got, want)
			}
		}
	}
}

func TestEntries_OS(t *testing.T) {
	root := t.TempDir()
	for name, content := range map[string]string{".gitignore": "*.log\n", "a/x.go": "", "a/y.log": "", "b/z.go": ""} {
		p := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	var got []string
	for e, err := range ignore.New().Entries(root, ignore.WalkOptions{}) {
		if err != nil {
			t.Fatal(err)
		}
		rel, relErr := filepath.Rel(root, e.Path)
		if relErr != nil {
			t.Fatal(relErr)
		}
		got = append(got, filepath.ToSlash(rel))
		if e.Name() == "b" {
			e.SkipDir()
		}
	}
	if want := []string{".gitignore", "a", "a/x.go", "b"}; !slices.Equal(got, want) {
		t.Errorf("Entries = %q, want %q", got, want)
	}
}

// SkipDir acts only on the directory being yielded: not on a file's
// siblings, and not through an Entry kept from an earlier iteration.
func TestEntries_SkipDirOnlyForTheCurrentDirectory(t *testing.T) {
	fsys := fstest.MapFS{"a/1": {}, "a/2": {}, "b/3": {}}
	var got []string
	var prev ignore.Entry
	for e, err := range ignore.New().EntriesFS(fsys, ".", ignore.WalkOptions{}) {
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, e.Path)
		if e.Path == "a/1" {
			prev.SkipDir() // prev is "a", whose loop body has returned
			e.SkipDir()    // a file
		}
		prev = e
	}
	if want := []string{"a", "a/1", "a/2", "b", "b/3"}; !slices.Equal(got, want) {
		t.Errorf("Entries = %q, want %q", got, want)
	}
}

func TestEntries_Break(t *testing.T) {
	fsys := fstest.MapFS{"a/1": {}, "a/2": {}, "b/3": {}}
	var got []string
	for e, err := range ignore.New().EntriesFS(fsys, ".", ignore.WalkOptions{}) {
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, e.Path)
		if len(got) == 2 {
			break
		}
	}
	if want := []string{"a", "a/1"}; !slices.Equal(got, want) {
		t.Errorf("Entries = %q, want %q", got, want)
	}
}

// readDirFailFS fails ReadDir for one directory.
type readDirFailFS struct {
	fstest.MapFS
	bad string
}

func (f readDirFailFS) ReadDir(name string) ([]fs.DirEntry, error) {
	if name == f.bad {
		return nil, errors.New("boom")
	}
	return f.MapFS.ReadDir(name)
}

// Errors come with their entry and the walk goes on; SkipDir on the error
// entry of a directory whose ignore file can't be read prunes it.
func TestEntries_Errors(t *testing.T) {
	fsys := readDirFailFS{MapFS: fstest.MapFS{
		"bad/x":          {},
		"c/1":            {},
		"sub/.gitignore": {Mode: fs.ModeNamedPipe},
		"sub/y":          {},
	}, bad: "bad"}
	type rec struct {
		path string
		err  bool
	}
	for _, skipSub := range []bool{false, true} {
		var got []rec
		for e, err := range ignore.New().EntriesFS(fsys, ".", ignore.WalkOptions{}) {
			got = append(got, rec{e.Path, err != nil})
			if err != nil && e.Path == "sub" {
				if !errors.Is(err, ignore.ErrPatternFileSkipped) {
					t.Errorf("sub: err = %v, want ErrPatternFileSkipped", err)
				}
				if skipSub {
					e.SkipDir()
				}
			}
		}
		want := []rec{{"bad", false}, {"bad", true}, {"c", false}, {"c/1", false}, {"sub", true}, {"sub/.gitignore", false}, {"sub/y", false}}
		if skipSub {
			want = want[:5]
		}
		if !slices.Equal(got, want) {
			t.Errorf("skipSub=%v: got %v, want %v", skipSub, got, want)
		}
	}

	var n int
	for e, err := range ignore.New().EntriesFS(fstest.MapFS{}, "nope", ignore.WalkOptions{}) {
		n++
		if e.Path != "nope" || !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("missing root: %q, %v", e.Path, err)
		}
	}
	if n != 1 {
		t.Errorf("missing root: %d yields, want 1", n)
	}
}

func TestRepoEntries(t *testing.T) {
	root := t.TempDir()
	for name, content := range map[string]string{
		".gitignore":        "*.log\n",
		".git/info/exclude": "secret\n",
		"a.go":              "",
		"b.log":             "",
		"secret":            "",
	} {
		p := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	var got []string
	for e, err := range ignore.RepoEntries(root, ignore.MatcherOptions{}, ignore.WalkOptions{}) {
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, e.Name())
	}
	if want := []string{".gitignore", "a.go"}; !slices.Equal(got, want) {
		t.Errorf("RepoEntries = %q, want %q", got, want)
	}
}
