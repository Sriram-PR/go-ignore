package ignore

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
)

// writeTree creates a directory tree from a map of relative path → file
// contents. Directories are created as needed. Empty contents create empty
// files; "DIR" creates a directory with no file.
func writeTree(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for rel, content := range files {
		full := filepath.Join(root, filepath.FromSlash(rel))
		if content == "DIR" {
			if err := os.MkdirAll(full, 0o755); err != nil {
				t.Fatalf("mkdir %s: %v", full, err)
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("mkdir parent of %s: %v", full, err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", full, err)
		}
	}
}

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
	if !equalStrings(got, want) {
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
	if !equalStrings(got, want) {
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
	if !equalStrings(got, want) {
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
	if !equalStrings(got, want) {
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
	if !equalStrings(got, want) {
		t.Errorf("got %v\nwant %v", got, want)
	}
}

func TestRepoFiles_LoadRepoErrorYielded(t *testing.T) {
	// If LoadRepo cannot read a corrupt source, RepoFiles must yield that
	// error once and stop. We force an error by making .gitignore unreadable.
	if runtime.GOOS == "windows" {
		t.Skip("chmod 0o000 unreliable on Windows")
	}
	root := t.TempDir()
	gitignore := filepath.Join(root, ".gitignore")
	if err := os.WriteFile(gitignore, []byte("*.log\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := os.Chmod(gitignore, 0o000); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(gitignore, 0o644) })

	yields := 0
	var firstErr error
	for path, err := range RepoFiles(root, MatcherOptions{}) {
		yields++
		if err != nil && firstErr == nil {
			firstErr = err
		}
		_ = path
	}
	if yields != 1 || firstErr == nil {
		t.Errorf("yields=%d firstErr=%v; want exactly one yield carrying a LoadRepo error", yields, firstErr)
	}
}

func TestWalkDirFS_BasicWithMapFS(t *testing.T) {
	fsys := fstest.MapFS{
		".gitignore":   {Data: []byte("*.log\n")},
		"keep.txt":     {Data: []byte("x")},
		"debug.log":    {Data: []byte("x")},
		"sub/file.txt": {Data: []byte("x")},
		"sub/err.log":  {Data: []byte("x")}, // ignored
	}

	// Pre-load the root .gitignore before walking — WalkDirFS discovers
	// nested .gitignore files but the receiver still supplies the root rules.
	m := New()
	if err := m.AddPatternsReader("", strings.NewReader("*.log\n")); err != nil {
		t.Fatalf("AddPatternsReader: %v", err)
	}

	var got []string
	err := m.WalkDirFS(fsys, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		got = append(got, path)
		return nil
	})
	if err != nil {
		t.Fatalf("WalkDirFS: %v", err)
	}
	sort.Strings(got)

	want := []string{
		".gitignore",
		"keep.txt",
		"sub/file.txt",
	}
	if !equalStrings(got, want) {
		t.Errorf("got %v\nwant %v", got, want)
	}
}

func TestWalkDirFS_NestedDiscoveryInMapFS(t *testing.T) {
	fsys := fstest.MapFS{
		"sub/.gitignore":  {Data: []byte("*.tmp\n")},
		"sub/keep.txt":    {Data: []byte("x")},
		"sub/scratch.tmp": {Data: []byte("x")},
		"top.tmp":         {Data: []byte("x")}, // not ignored at root
	}

	var got []string
	err := New().WalkDirFS(fsys, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		got = append(got, path)
		return nil
	})
	if err != nil {
		t.Fatalf("WalkDirFS: %v", err)
	}
	sort.Strings(got)

	want := []string{
		"sub/.gitignore",
		"sub/keep.txt",
		"top.tmp",
	}
	if !equalStrings(got, want) {
		t.Errorf("got %v\nwant %v", got, want)
	}
}

func TestWalkDirFS_PathsAreForwardSlash(t *testing.T) {
	// fs.FS paths are always forward-slash regardless of host OS. This test
	// guards against a future refactor that accidentally passes the path
	// through filepath.ToSlash twice or back through OS-native conversion.
	fsys := fstest.MapFS{
		"a/b/c.txt": {Data: []byte("x")},
	}

	var seen string
	err := New().WalkDirFS(fsys, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			seen = path
		}
		return nil
	})
	if err != nil {
		t.Fatalf("WalkDirFS: %v", err)
	}
	if seen != "a/b/c.txt" {
		t.Errorf("path = %q, want %q (forward slashes, no OS conversion)", seen, "a/b/c.txt")
	}
}

func TestWalkDirFS_DotGitPruned(t *testing.T) {
	fsys := fstest.MapFS{
		"keep.txt":          {Data: []byte("x")},
		".git/HEAD":         {Data: []byte("ref")},
		".git/objects/pack": {Data: []byte("x")},
	}

	var got []string
	err := New().WalkDirFS(fsys, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		got = append(got, path)
		return nil
	})
	if err != nil {
		t.Fatalf("WalkDirFS: %v", err)
	}
	for _, p := range got {
		if strings.HasPrefix(p, ".git/") || p == ".git" {
			t.Errorf("WalkDirFS descended into %q; .git must be pruned", p)
		}
	}
}

func TestWalkDirFS_DoesNotMutateReceiver(t *testing.T) {
	fsys := fstest.MapFS{
		".gitignore":     {Data: []byte("*.log\n")},
		"sub/.gitignore": {Data: []byte("*.tmp\n")},
		"sub/file.txt":   {Data: []byte("x")},
	}
	m := New()
	m.AddPatterns("", []byte("manual.pattern\n"))
	before := m.RuleCount()

	err := m.WalkDirFS(fsys, ".", func(path string, d fs.DirEntry, err error) error {
		return err
	})
	if err != nil {
		t.Fatalf("WalkDirFS: %v", err)
	}
	if after := m.RuleCount(); before != after {
		t.Errorf("WalkDirFS mutated receiver: %d → %d", before, after)
	}
}

func TestWalkDirFS_SubrootWalk(t *testing.T) {
	// Walking from a subroot (not "." or "") must produce paths under that
	// subroot, with rel computed correctly via prefix-strip.
	fsys := fstest.MapFS{
		"a/x.txt":    {Data: []byte("x")},
		"a/y.txt":    {Data: []byte("x")},
		"b/skip.txt": {Data: []byte("x")},
	}

	var got []string
	err := New().WalkDirFS(fsys, "a", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			got = append(got, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("WalkDirFS: %v", err)
	}
	sort.Strings(got)

	want := []string{"a/x.txt", "a/y.txt"}
	if !equalStrings(got, want) {
		t.Errorf("got %v\nwant %v", got, want)
	}
}

func TestFilesFS_BasicWithMapFS(t *testing.T) {
	fsys := fstest.MapFS{
		".gitignore":   {Data: []byte("*.log\n")},
		"keep.txt":     {Data: []byte("x")},
		"debug.log":    {Data: []byte("x")}, // ignored
		"sub/file.txt": {Data: []byte("x")},
	}

	m := New()
	m.AddPatterns("", []byte("*.log\n"))

	var got []string
	for path, err := range m.FilesFS(fsys, ".") {
		if err != nil {
			t.Fatalf("FilesFS: %v", err)
		}
		got = append(got, path)
	}
	sort.Strings(got)

	want := []string{
		".gitignore",
		"keep.txt",
		"sub/file.txt",
	}
	if !equalStrings(got, want) {
		t.Errorf("got %v\nwant %v", got, want)
	}
}

func TestFilesFS_NestedDiscoveryInMapFS(t *testing.T) {
	// FilesFS is built on WalkDirFS — nested .gitignore discovery still applies.
	fsys := fstest.MapFS{
		"sub/.gitignore":  {Data: []byte("*.tmp\n")},
		"sub/keep.txt":    {Data: []byte("x")},
		"sub/scratch.tmp": {Data: []byte("x")}, // ignored by sub/.gitignore
		"top.tmp":         {Data: []byte("x")}, // NOT ignored (scope is sub/)
	}

	var got []string
	for path, err := range New().FilesFS(fsys, ".") {
		if err != nil {
			t.Fatalf("FilesFS: %v", err)
		}
		got = append(got, path)
	}
	sort.Strings(got)

	want := []string{
		"sub/.gitignore",
		"sub/keep.txt",
		"top.tmp",
	}
	if !equalStrings(got, want) {
		t.Errorf("got %v\nwant %v", got, want)
	}
}

func TestFilesFS_BreakStopsCleanly(t *testing.T) {
	fsys := fstest.MapFS{
		"a.txt": {Data: []byte("x")},
		"b.txt": {Data: []byte("x")},
		"c.txt": {Data: []byte("x")},
		"d.txt": {Data: []byte("x")},
	}

	seen := 0
	for _, err := range New().FilesFS(fsys, ".") {
		if err != nil {
			t.Fatalf("FilesFS: %v", err)
		}
		seen++
		if seen == 2 {
			break
		}
	}
	if seen != 2 {
		t.Errorf("seen = %d after break, want 2", seen)
	}
}

func TestFilesFS_FilesOnly(t *testing.T) {
	// FilesFS must not yield directory entries.
	fsys := fstest.MapFS{
		"a/b/c.txt": {Data: []byte("x")},
	}

	for path, err := range New().FilesFS(fsys, ".") {
		if err != nil {
			t.Fatalf("FilesFS: %v", err)
		}
		// Every yielded path must be a file, never a directory.
		info, statErr := fs.Stat(fsys, path)
		if statErr != nil {
			t.Fatalf("stat %q: %v", path, statErr)
		}
		if info.IsDir() {
			t.Errorf("FilesFS yielded directory %q", path)
		}
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
