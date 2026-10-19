package ignore

import (
	"errors"
	"fmt"
	"io/fs"
	"sync"
	"testing"
	"testing/fstest"
)

// countingFS counts the opens of each file, so tests can tell which
// .gitignore files were read and how often.
type countingFS struct {
	fstest.MapFS
	mu    sync.Mutex
	opens map[string]int
}

func newCountingFS(m fstest.MapFS) *countingFS {
	return &countingFS{MapFS: m, opens: map[string]int{}}
}

func (c *countingFS) Open(name string) (fs.File, error) {
	c.mu.Lock()
	c.opens[name]++
	c.mu.Unlock()
	return c.MapFS.Open(name)
}

func (c *countingFS) count(name string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.opens[name]
}

func file(s string) *fstest.MapFile { return &fstest.MapFile{Data: []byte(s)} }

var dirEntry = &fstest.MapFile{Mode: fs.ModeDir | 0o755}

func mustCheck(t *testing.T, tr *Tree, path string) MatchResult {
	t.Helper()
	res, err := tr.Check(path)
	if err != nil {
		t.Fatalf("Check(%q): unexpected error %v", path, err)
	}
	return res
}

// A nested .gitignore applies below its directory and nowhere else, and
// MatchResult names the file and line that decided.
func TestTree_NestedRules(t *testing.T) {
	fsys := fstest.MapFS{
		".gitignore":     file("*.log\n"),
		"src/.gitignore": file("*.tmp\n!keep.log\n"),
		"src/x.tmp":      file(""),
		"src/keep.log":   file(""),
		"src/a/b.tmp":    file(""),
		"lib/x.tmp":      file(""),
	}
	tr := New().TreeFS(fsys, ".")

	res := mustCheck(t, tr, "src/x.tmp")
	if !res.Ignored || res.Source != "src/.gitignore" || res.Line != 1 || res.BasePath != "src" || res.Rule != "*.tmp" {
		t.Errorf("src/x.tmp: %+v, want ignored by src/.gitignore:1 *.tmp", res)
	}
	if res := mustCheck(t, tr, "src/keep.log"); res.Ignored || !res.Matched || res.Rule != "!keep.log" {
		t.Errorf("src/keep.log: %+v, want re-included by !keep.log", res)
	}
	if res := mustCheck(t, tr, "src/a/b.tmp"); !res.Ignored {
		t.Errorf("src/a/b.tmp: %+v, want ignored by src/.gitignore", res)
	}
	if res := mustCheck(t, tr, "lib/x.tmp"); res.Matched {
		t.Errorf("lib/x.tmp: %+v, want no match (src/.gitignore is a sibling's)", res)
	}
}

// An ignored parent decides, the outermost one is reported, and no
// .gitignore inside it is read, as in git.
func TestTree_IgnoredParentNotRead(t *testing.T) {
	fsys := newCountingFS(fstest.MapFS{
		".gitignore":       file("a/\n"),
		"a/.gitignore":     file("!*\n"),
		"a/b/.gitignore":   file("!*\n"),
		"a/b/c":            file(""),
		"keep/.gitignore":  file("x\n"),
		"keep/sub/file.go": file(""),
	})
	tr := New().TreeFS(fsys, ".")
	if res := mustCheck(t, tr, "a/b/c"); !res.Ignored || res.Rule != "a/" || res.BasePath != "" {
		t.Errorf("a/b/c: %+v, want ignored by the root rule a/", res)
	}
	for _, name := range []string{"a/.gitignore", "a/b/.gitignore"} {
		if n := fsys.count(name); n != 0 {
			t.Errorf("%s opened %d times, want 0 (inside an ignored directory)", name, n)
		}
	}
	mustCheck(t, tr, "keep/sub/file.go")
	if n := fsys.count("keep/.gitignore"); n != 1 {
		t.Errorf("keep/.gitignore opened %d times, want 1", n)
	}
}

// Check decides isDir as git does: a trailing slash, else lstat, where a
// symlink or a missing path is a file. Parents are always directories.
func TestTree_CheckDecidesIsDir(t *testing.T) {
	fsys := fstest.MapFS{
		".gitignore": file("build/\nlink/\ngone/\n"),
		"build":      dirEntry,
		"link":       &fstest.MapFile{Data: []byte("build"), Mode: fs.ModeSymlink},
	}
	tr := New().TreeFS(fsys, ".")
	for _, tc := range []struct {
		path string
		want bool
	}{
		{"build", true},
		{"link", false},
		{"gone", false},
		{"gone/", true},
		{"gone/x", true},
	} {
		if got := mustCheck(t, tr, tc.path).Ignored; got != tc.want {
			t.Errorf("Check(%q).Ignored = %v, want %v", tc.path, got, tc.want)
		}
	}
	if res, _ := tr.CheckIsDir("gone", true); !res.Ignored {
		t.Error("CheckIsDir(gone, true) not ignored by gone/")
	}
	if res, _ := tr.CheckIsDir("build", false); res.Ignored {
		t.Error("CheckIsDir(build, false) ignored; the caller's isDir must win")
	}
}

// A .gitignore that can't be read gives git's answer without it, plus an
// error, on every check that consults its directory.
func TestTree_UnreadableGitignore(t *testing.T) {
	fsys := fstest.MapFS{
		".gitignore":      file("*.log\nskip/\n"),
		"src/.gitignore":  dirEntry,
		"lib/.gitignore":  &fstest.MapFile{Data: []byte("../x"), Mode: fs.ModeSymlink},
		"skip/.gitignore": dirEntry,
	}
	tr := New().TreeFS(fsys, ".")
	for range 2 {
		res, err := tr.Check("src/x.log")
		if !res.Ignored || !errors.Is(err, ErrPatternFileSkipped) {
			t.Errorf("src/x.log: %+v, %v; want ignored by *.log and ErrPatternFileSkipped", res, err)
		}
	}
	if _, err := tr.Check("lib/x"); !errors.Is(err, ErrPatternFileSkipped) {
		t.Errorf("lib/x: err = %v, want ErrPatternFileSkipped (symlinked .gitignore)", err)
	}
	if _, err := tr.Check("x.log"); err != nil {
		t.Errorf("x.log: err = %v, want nil (src not consulted)", err)
	}
	if res, err := tr.Check("skip/x"); !res.Ignored || err != nil {
		t.Errorf("skip/x: %+v, %v; want ignored by skip/ with no error (never read)", res, err)
	}
}

// A Tree keeps what it read and what the matcher held when it was made.
func TestTree_Snapshot(t *testing.T) {
	fsys := fstest.MapFS{
		"src/.gitignore": file("a\n"),
	}
	m := New()
	tr := m.TreeFS(fsys, ".")
	mustCheck(t, tr, "src/a")
	fsys["src/.gitignore"] = file("b\n")
	m.AddPatterns("", []byte("c\n"))
	if !mustCheck(t, tr, "src/a").Ignored || mustCheck(t, tr, "src/b").Ignored || mustCheck(t, tr, "c").Ignored {
		t.Error("the Tree saw an edit made after it read the file or was made")
	}
	tr2 := m.TreeFS(fsys, ".")
	if !mustCheck(t, tr2, "src/b").Ignored || !mustCheck(t, tr2, "c").Ignored {
		t.Error("a new Tree does not see the edits")
	}
}

// MaxPatterns counts the snapshot and the ancestor chain, so a nested file
// is truncated at the same rule as in the walker.
func TestTree_MaxPatternsAlongChain(t *testing.T) {
	fsys := fstest.MapFS{
		".gitignore":     file("a\nb\n"),
		"src/.gitignore": file("c\nd\n"),
		"lib/.gitignore": file("e\n"),
		"src/c":          file(""),
		"src/d":          file(""),
		"lib/e":          file(""),
	}
	var warnings []string
	m := NewWithOptions(MatcherOptions{
		MaxPatterns:    4,
		WarningHandler: func(w ParseWarning) { warnings = append(warnings, w.BasePath+": "+w.Message) },
	})
	m.AddPatterns("", []byte("base\n"))
	tr := m.TreeFS(fsys, ".")
	for path, want := range map[string]bool{"src/c": true, "src/d": false, "lib/e": true} {
		if got := mustCheck(t, tr, path).Ignored; got != want {
			t.Errorf("Check(%q).Ignored = %v, want %v", path, got, want)
		}
	}
	if len(warnings) != 1 || warnings[0] != "src: maximum pattern count reached, excess patterns truncated" {
		t.Errorf("warnings = %q, want one truncation warning for src", warnings)
	}

	walked := map[string]bool{}
	err := m.WalkDirFS(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		walked[p] = true
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string]bool{"src/c": true, "src/d": false, "lib/e": true} {
		if walked[path] == want {
			t.Errorf("walker: %s visited = %v, but Tree says ignored = %v", path, walked[path], want)
		}
	}
}

// Under CaseInsensitive, a nested file in a mixed-case directory applies.
func TestTree_CaseInsensitive(t *testing.T) {
	fsys := fstest.MapFS{
		"Src/.gitignore": file("*.TMP\n"),
		"Src/X.tmp":      file(""),
	}
	tr := NewWithOptions(MatcherOptions{CaseInsensitive: true}).TreeFS(fsys, ".")
	if res := mustCheck(t, tr, "Src/X.tmp"); !res.Ignored || res.BasePath != "Src" {
		t.Errorf("Src/X.tmp: %+v, want ignored by Src/.gitignore", res)
	}
}

// Paths that match nothing in Match match nothing here, without an error.
func TestTree_OutOfRangePaths(t *testing.T) {
	tr := NewWithOptions(MatcherOptions{}).TreeFS(fstest.MapFS{".gitignore": file("*\n")}, ".")
	for _, p := range []string{"", "..", "../x", "a/../.."} {
		if res, err := tr.Check(p); res.Matched || err != nil {
			t.Errorf("Check(%q) = %+v, %v; want no match and no error", p, res, err)
		}
	}
}

// Parse warnings from nested files go to the handler, which may call back
// into the Tree; the matcher's own warnings are untouched.
func TestTree_WarningsToHandler(t *testing.T) {
	fsys := fstest.MapFS{"src/.gitignore": file("!\nx\n")}
	var tr *Tree
	var got []ParseWarning
	m := NewWithOptions(MatcherOptions{WarningHandler: func(w ParseWarning) {
		got = append(got, w)
		if _, err := tr.Check("src/y"); err != nil {
			t.Errorf("Check from the handler: %v", err)
		}
	}})
	tr = m.TreeFS(fsys, ".")
	if !mustCheck(t, tr, "src/x").Ignored {
		t.Error("src/x not ignored")
	}
	if len(got) != 1 || got[0].BasePath != "src" || got[0].Line != 1 {
		t.Errorf("handler got %+v, want one warning for src line 1", got)
	}
	if w := m.Warnings(); len(w) != 0 {
		t.Errorf("matcher Warnings = %v, want none", w)
	}
}

// Concurrent checks are safe and still read each .gitignore once.
func TestTree_ConcurrentReadsOnce(t *testing.T) {
	files := fstest.MapFS{}
	for i := range 20 {
		files[fmt.Sprintf("d%d/.gitignore", i)] = file("*.tmp\n")
	}
	fsys := newCountingFS(files)
	tr := New().TreeFS(fsys, ".")
	var wg sync.WaitGroup
	for range 16 {
		wg.Go(func() {
			for i := range 20 {
				p := fmt.Sprintf("d%d/sub/x.tmp", i)
				if res, err := tr.CheckIsDir(p, false); !res.Ignored || err != nil {
					t.Errorf("%s: %+v, %v", p, res, err)
				}
			}
		})
	}
	wg.Wait()
	for i := range 20 {
		if n := fsys.count(fmt.Sprintf("d%d/.gitignore", i)); n != 1 {
			t.Errorf("d%d/.gitignore opened %d times, want 1", i, n)
		}
	}
}
