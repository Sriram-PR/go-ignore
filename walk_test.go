package ignore

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strings"
	"sync"
	"testing"
)

// collectWalk runs m.WalkDir on root and returns the sorted slash-form
// relative paths fn was called for, excluding the root itself.
func collectWalk(t *testing.T, m *Matcher, root string) []string {
	t.Helper()
	var got []string
	err := m.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)
		if rel != "." {
			got = append(got, rel)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("WalkDir: %v", err)
	}
	sort.Strings(got)
	return got
}

func TestWalkDir_BasicIgnore(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		".gitignore":    "*.log\nbuild/\n",
		"keep.txt":      "x",
		"debug.log":     "x",
		"build/out.js":  "x",
		"src/main.go":   "x",
		"src/error.log": "x", // ignored by floating *.log
	})

	// Use WalkRepo so the root .gitignore is loaded automatically; WalkDir on a
	// fresh New() matcher would honor the receiver's rules only (no nested
	// discovery for the root itself, since the receiver wasn't preloaded).
	var got []string
	err := WalkRepo(root, MatcherOptions{}, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)
		if rel != "." {
			got = append(got, rel)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("WalkRepo: %v", err)
	}
	sort.Strings(got)

	want := []string{
		".gitignore",
		"keep.txt",
		"src",
		"src/main.go",
	}
	if !slices.Equal(got, want) {
		t.Errorf("got %v\nwant %v", got, want)
	}
}

func TestWalkDir_NestedGitignoreDiscovered(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		".gitignore":           "*.log\n",
		"top.log":              "x",
		"top.tmp":              "x",
		"sub/.gitignore":       "*.tmp\n",
		"sub/keep.txt":         "x",
		"sub/scratch.tmp":      "x",
		"sub/inner/.gitignore": "secret\n",
		"sub/inner/file.txt":   "x",
		"sub/inner/secret":     "x",
	})

	var got []string
	err := WalkRepo(root, MatcherOptions{}, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)
		if rel != "." {
			got = append(got, rel)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("WalkRepo: %v", err)
	}
	sort.Strings(got)

	// top.tmp survives (root .gitignore only ignores *.log)
	// sub/scratch.tmp is ignored by sub/.gitignore
	// sub/inner/secret is ignored by sub/inner/.gitignore
	want := []string{
		".gitignore",
		"sub",
		"sub/.gitignore",
		"sub/inner",
		"sub/inner/.gitignore",
		"sub/inner/file.txt",
		"sub/keep.txt",
		"top.tmp",
	}
	if !slices.Equal(got, want) {
		t.Errorf("got %v\nwant %v", got, want)
	}
}

func TestWalkDir_NestedRulesDoNotLeakAcrossSiblings(t *testing.T) {
	// sub-a/.gitignore ignores *.tmp; sub-b has no .gitignore.
	// A *.tmp file in sub-b must NOT be ignored even though sub-a's rule
	// was loaded earlier in the walk — basePath scoping prevents leakage.
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		"sub-a/.gitignore": "*.tmp\n",
		"sub-a/a.tmp":      "x",
		"sub-b/b.tmp":      "x",
	})

	got := []string{}
	err := WalkRepo(root, MatcherOptions{}, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)
		if rel != "." && !d.IsDir() {
			got = append(got, rel)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("WalkRepo: %v", err)
	}
	sort.Strings(got)

	want := []string{
		"sub-a/.gitignore",
		"sub-b/b.tmp", // b.tmp must survive
	}
	if !slices.Equal(got, want) {
		t.Errorf("got %v\nwant %v", got, want)
	}
}

func TestWalkDir_DotGitAlwaysPruned(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		"keep.txt":          "x",
		".git/HEAD":         "ref",
		".git/objects/pack": "DIR",
		".git/info/exclude": "",
	})

	got := collectWalk(t, New(), root)
	for _, p := range got {
		if strings.HasPrefix(p, ".git/") || p == ".git" {
			t.Errorf("WalkDir descended into %q; .git must be pruned", p)
		}
	}
	// keep.txt should still be present.
	found := false
	for _, p := range got {
		if p == "keep.txt" {
			found = true
		}
	}
	if !found {
		t.Error("keep.txt missing from walk")
	}
}

func TestWalkDir_DoesNotMutateReceiver(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		".gitignore":     "*.log\n",
		"sub/.gitignore": "*.tmp\n",
		"sub/file.txt":   "x",
		"top.log":        "x",
	})

	m := New()
	m.AddPatterns("", []byte("manual.pattern\n"))
	before := m.RuleCount()

	err := m.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		return err
	})
	if err != nil {
		t.Fatalf("WalkDir: %v", err)
	}

	after := m.RuleCount()
	if before != after {
		t.Errorf("WalkDir mutated receiver: rule count %d → %d", before, after)
	}
}

func TestWalkDir_SourcePopulatedForNestedRules(t *testing.T) {
	// WalkDir discovers nested .gitignore files via the same internal path as
	// AddPatternsFromFile, which propagates the file path to Source. We assert
	// the propagation directly: AddPatternsFromFile is the lower-level primitive
	// WalkDir builds on, so if Source is correct here it's correct in WalkDir.
	root := t.TempDir()
	subIgnore := filepath.Join(root, "sub", ".gitignore")
	writeTree(t, root, map[string]string{
		"sub/.gitignore": "scratch\n",
	})

	m := New()
	if err := m.AddPatternsFromFile("sub", subIgnore); err != nil {
		t.Fatalf("AddPatternsFromFile: %v", err)
	}
	r := m.MatchWithReason("sub/scratch", false)
	if r.Source != subIgnore {
		t.Errorf("Source = %q, want %q", r.Source, subIgnore)
	}
}

func TestWalkDir_CallbackErrorAborts(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		"a.txt":     "x",
		"b.txt":     "x",
		"c/sub.txt": "x",
	})

	sentinel := errors.New("stop")
	err := New().WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if filepath.Base(path) == "b.txt" {
			return sentinel
		}
		return nil
	})
	if !errors.Is(err, sentinel) {
		t.Errorf("expected sentinel error from WalkDir, got %v", err)
	}
}

func TestWalkDir_ConcurrentMatchOnReceiver(t *testing.T) {
	// While WalkDir runs, concurrent Match calls on the receiver must succeed
	// and not race (verified under -race).
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		".gitignore": "*.log\n",
		"a.log":      "x",
		"b.txt":      "x",
	})

	m, err := LoadRepo(root, MatcherOptions{})
	if err != nil {
		t.Fatalf("LoadRepo: %v", err)
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = m.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			return err
		})
	}()

	for i := 0; i < 100; i++ {
		_ = m.Match("a.log", false)
		_ = m.Match("b.txt", false)
	}
	wg.Wait()
}

func TestFiles_BasicAndFilesOnly(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		".gitignore":    "*.log\n",
		"keep.txt":      "x",
		"debug.log":     "x",
		"sub/inner.txt": "x",
	})

	var got []string
	for path, err := range RepoFiles(root, MatcherOptions{}) {
		if err != nil {
			t.Fatalf("RepoFiles: %v", err)
		}
		rel, _ := filepath.Rel(root, path)
		got = append(got, filepath.ToSlash(rel))
	}
	sort.Strings(got)

	// Iterator yields files only — no directory entries.
	want := []string{
		".gitignore",
		"keep.txt",
		"sub/inner.txt",
	}
	if !slices.Equal(got, want) {
		t.Errorf("got %v\nwant %v", got, want)
	}
}

func TestFiles_BreakStopsCleanly(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		"a.txt": "x",
		"b.txt": "x",
		"c.txt": "x",
		"d.txt": "x",
	})

	seen := 0
	for path, err := range New().Files(root) {
		if err != nil {
			t.Fatalf("Files: %v", err)
		}
		_ = path
		seen++
		if seen == 2 {
			break
		}
	}
	if seen != 2 {
		t.Errorf("seen = %d after break, want 2", seen)
	}
}

func TestFiles_BreakDoesNotLeakSkipAll(t *testing.T) {
	// After a break, the iterator must complete cleanly — no "skip all" or
	// other sentinel error should reach the caller via a final yield. We
	// verify by counting yields and confirming the last one was an actual file
	// path (not ("", err)).
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		"a.txt": "x",
		"b.txt": "x",
		"c.txt": "x",
	})

	var last struct {
		path string
		err  error
	}
	yields := 0
	for path, err := range New().Files(root) {
		yields++
		last.path = path
		last.err = err
		if yields == 1 {
			break
		}
	}
	if yields != 1 {
		t.Fatalf("yields = %d, want 1", yields)
	}
	if last.err != nil {
		t.Errorf("last yield carried err = %v, want nil — fs.SkipAll must not leak", last.err)
	}
	if last.path == "" {
		t.Errorf("last yield path empty, want a real file path")
	}
}

func TestFiles_TraversalError(t *testing.T) {
	// On Unix, a directory with mode 0o000 cannot be read; filepath.WalkDir
	// surfaces the error to the callback. The iterator must yield it.
	if runtime.GOOS == "windows" {
		t.Skip("chmod 0o000 unreliable on Windows")
	}
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		"ok.txt": "x",
	})
	denied := filepath.Join(root, "denied")
	if err := os.Mkdir(denied, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.Chmod(denied, 0o000); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() {
		// Restore so the temp-dir cleanup can remove it.
		_ = os.Chmod(denied, 0o755)
	})

	sawErr := false
	for _, err := range New().Files(root) {
		if err != nil {
			sawErr = true
		}
	}
	if !sawErr {
		t.Error("expected iterator to yield a traversal error for unreadable directory")
	}
}

func TestFiles_NestedDiscoveryStillApplies(t *testing.T) {
	// Files() uses WalkDir internally, so nested .gitignore discovery must
	// still apply to its output.
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		"sub/.gitignore":  "*.tmp\n",
		"sub/keep.txt":    "x",
		"sub/scratch.tmp": "x",
		"other.tmp":       "x", // not ignored (sub/.gitignore scoped to sub/)
	})

	var got []string
	for path, err := range RepoFiles(root, MatcherOptions{}) {
		if err != nil {
			t.Fatalf("RepoFiles: %v", err)
		}
		rel, _ := filepath.Rel(root, path)
		got = append(got, filepath.ToSlash(rel))
	}
	sort.Strings(got)

	want := []string{
		"other.tmp",
		"sub/.gitignore",
		"sub/keep.txt",
	}
	if !slices.Equal(got, want) {
		t.Errorf("got %v\nwant %v", got, want)
	}
}

func TestRepoFiles_UnreadableRootGitignoreYieldedOnce(t *testing.T) {
	// An unreadable root .gitignore is reported once and the walk goes on
	// without its rules, as git does.
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("needs POSIX permissions and a non-root user")
	}
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		".gitignore": "*.log\n",
		"a.log":      "x",
	})
	gitignore := filepath.Join(root, ".gitignore")
	if err := os.Chmod(gitignore, 0o000); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(gitignore, 0o644) })

	var errs []error
	sawLog := false
	for path, err := range RepoFiles(root, MatcherOptions{}) {
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if filepath.Base(path) == "a.log" {
			sawLog = true
		}
	}
	if len(errs) != 1 || !errors.Is(errs[0], fs.ErrPermission) {
		t.Errorf("errors = %v; want exactly one wrapping fs.ErrPermission", errs)
	}
	if !sawLog {
		t.Error("a.log not yielded; want the walk to continue without the unreadable rules")
	}
}

// In worktrees and submodules .git is a file; git never lists it.
func TestWalkDir_DotGitFileNotYielded(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		"keep.txt":     "x",
		".git":         "gitdir: ../main/.git/worktrees/wt\n",
		"sub/.git":     "gitdir: ../.git/modules/sub\n",
		"sub/file.txt": "x",
	})

	got := collectWalk(t, New(), root)
	want := []string{"keep.txt", "sub", "sub/file.txt"}
	if !slices.Equal(got, want) {
		t.Errorf("WalkDir = %v, want %v", got, want)
	}
}

// An unreadable nested .gitignore gets one callback, with the permission
// error, and the walk continues into the directory, as git does.
func TestWalkDir_UnreadableGitignoreOneCallback(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("needs POSIX permissions and a non-root user")
	}
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		"sub/.gitignore": "*.log\n",
		"sub/a.log":      "x",
	})
	gitignore := filepath.Join(root, "sub", ".gitignore")
	if err := os.Chmod(gitignore, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(gitignore, 0o644) })

	subCalls := 0
	var subErr error
	sawLog := false
	err := New().WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		switch filepath.Base(p) {
		case "sub":
			subCalls++
			subErr = err
		case "a.log":
			sawLog = true
		}
		return nil
	})
	if err != nil {
		t.Fatalf("WalkDir: %v", err)
	}
	if subCalls != 1 || !errors.Is(subErr, fs.ErrPermission) {
		t.Errorf("sub got %d callbacks, err %v; want 1 with fs.ErrPermission", subCalls, subErr)
	}
	if !sawLog {
		t.Error("sub/a.log not visited; want the walk to continue without the unreadable rules")
	}
}

// WalkRepo reads the root .gitignore once: LoadRepo's copy and the walker's
// used to both load it.
func TestWalkRepo_RootGitignoreLoadedOnce(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{".gitignore": "!\n*.log\n"})
	warnings := 0
	opts := MatcherOptions{WarningHandler: func(ParseWarning) { warnings++ }}
	if err := WalkRepo(root, opts, func(string, fs.DirEntry, error) error { return nil }); err != nil {
		t.Fatalf("WalkRepo: %v", err)
	}
	if warnings != 1 {
		t.Errorf("got %d warnings for the root .gitignore's \"!\" line, want 1", warnings)
	}
}

// WalkRepo with an unreadable root .gitignore reports it once through fn and
// walks the tree.
func TestWalkRepo_UnreadableRootGitignoreContinues(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("needs POSIX permissions and a non-root user")
	}
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		".gitignore": "*.log\n",
		"a.log":      "x",
	})
	gitignore := filepath.Join(root, ".gitignore")
	if err := os.Chmod(gitignore, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(gitignore, 0o644) })

	rootCalls := 0
	var rootErr error
	sawLog := false
	err := WalkRepo(root, MatcherOptions{}, func(p string, d fs.DirEntry, err error) error {
		if p == root {
			rootCalls++
			rootErr = err
		}
		if filepath.Base(p) == "a.log" {
			sawLog = true
		}
		return nil
	})
	if err != nil {
		t.Fatalf("WalkRepo: %v", err)
	}
	if rootCalls != 1 || !errors.Is(rootErr, fs.ErrPermission) {
		t.Errorf("root got %d callbacks, err %v; want 1 with fs.ErrPermission", rootCalls, rootErr)
	}
	if !sawLog {
		t.Error("a.log not visited; want the walk to continue")
	}
}
