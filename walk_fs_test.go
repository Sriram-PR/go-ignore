package ignore

import (
	"errors"
	"fmt"
	"io/fs"
	"slices"
	"sort"
	"strings"
	"testing"
	"testing/fstest"
)

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
	if !slices.Equal(got, want) {
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
	if !slices.Equal(got, want) {
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
	if !slices.Equal(got, want) {
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
	if !slices.Equal(got, want) {
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
	if !slices.Equal(got, want) {
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

// With the allowlist idiom, re-included directories are walked but only the
// files a rule re-includes are yielded, as git ls-files does.
func TestWalkDirFS_AllowlistIdiom(t *testing.T) {
	fsys := fstest.MapFS{
		".gitignore":    {Data: []byte("*\n!.gitignore\n!*/\n!*.go\n")},
		"root.txt":      {Data: []byte("x")},
		"pkg/a.txt":     {Data: []byte("x")},
		"pkg/main.go":   {Data: []byte("x")},
		"pkg/sub/b.bin": {Data: []byte("x")},
		"pkg/sub/c.go":  {Data: []byte("x")},
	}
	var got []string
	for p, err := range New().FilesFS(fsys, ".") {
		if err != nil {
			t.Fatalf("FilesFS: %v", err)
		}
		got = append(got, p)
	}
	sort.Strings(got)
	want := []string{".gitignore", "pkg/main.go", "pkg/sub/c.go"}
	if !slices.Equal(got, want) {
		t.Errorf("got %v\nwant %v", got, want)
	}
}

// Rules from a sibling directory's .gitignore stop counting toward
// MaxPatterns once the walk leaves that directory, so every directory's own
// rules apply even when the tree's total exceeds the cap. Each directory's
// rules stay scoped to it.
func TestWalkDirFS_RuleStackScopesMaxPatterns(t *testing.T) {
	fsys := fstest.MapFS{}
	var want []string
	for i := range 20 {
		dir := fmt.Sprintf("d%02d", i)
		rules := fmt.Sprintf("r%d-1\nr%d-2\nr%d-3\nr%d-4\n*.tmp\n", i, i, i, i)
		if i == 0 {
			rules = "r0-1\nr0-2\nr0-3\ny.txt\n*.tmp\n"
		}
		fsys[dir+"/.gitignore"] = &fstest.MapFile{Data: []byte(rules)}
		fsys[dir+"/x.tmp"] = &fstest.MapFile{}
		fsys[dir+"/y.txt"] = &fstest.MapFile{}
		want = append(want, dir+"/.gitignore")
		if i != 0 {
			want = append(want, dir+"/y.txt")
		}
	}
	var warnings []ParseWarning
	m := NewWithOptions(MatcherOptions{
		MaxPatterns:    30,
		WarningHandler: func(w ParseWarning) { warnings = append(warnings, w) },
	})
	var got []string
	for p, err := range m.FilesFS(fsys, ".") {
		if err != nil {
			t.Fatalf("FilesFS: %v", err)
		}
		got = append(got, p)
	}
	sort.Strings(got)
	sort.Strings(want)
	if !slices.Equal(got, want) {
		t.Errorf("got %v\nwant %v", got, want)
	}
	if len(warnings) != 0 {
		t.Errorf("warnings = %v, want none", warnings)
	}
}

// A .gitignore the walker refuses to read gets exactly one callback for its
// directory, carrying ErrPatternFileSkipped. fn's return decides what
// happens next: nil walks the directory without that file's rules (git's
// choice), SkipDir prunes it.
func TestWalkDirFS_RefusedGitignoreOneCallback(t *testing.T) {
	kinds := map[string]*fstest.MapFile{
		"symlink": {Mode: fs.ModeSymlink, Data: []byte("../rules")},
		"pipe":    {Mode: fs.ModeNamedPipe},
		"dir":     {Mode: fs.ModeDir | 0o755},
	}
	for name, gitignore := range kinds {
		for _, ret := range []error{nil, fs.SkipDir} {
			fsys := fstest.MapFS{
				"rules":          {Data: []byte("*.log\n")},
				"sub/.gitignore": gitignore,
				"sub/a.log":      {},
				"sub/b.txt":      {},
			}
			subCalls := 0
			var subErr error
			var files []string
			err := New().WalkDirFS(fsys, ".", func(p string, d fs.DirEntry, err error) error {
				if p == "sub" {
					subCalls++
					subErr = err
					if err != nil {
						return ret
					}
				}
				if err == nil && !d.IsDir() {
					files = append(files, p)
				}
				return nil
			})
			if err != nil {
				t.Fatalf("%s: WalkDirFS: %v", name, err)
			}
			if subCalls != 1 || !errors.Is(subErr, ErrPatternFileSkipped) {
				t.Errorf("%s: sub got %d callbacks, err %v; want 1 with ErrPatternFileSkipped", name, subCalls, subErr)
			}
			sort.Strings(files)
			// A symlink or pipe named .gitignore is itself a file the walk
			// yields, as git lists it.
			want := []string{"rules", "sub/a.log", "sub/b.txt"}
			if !gitignore.Mode.IsDir() {
				want = append(want, "sub/.gitignore")
			}
			if ret == fs.SkipDir {
				want = []string{"rules"}
			}
			sort.Strings(want)
			if !slices.Equal(files, want) {
				t.Errorf("%s, fn returns %v: files %v, want %v", name, ret, files, want)
			}
		}
	}
}

// FilesFS yields regular files and symlinks only, as git ls-files lists them;
// EntriesFS passes special files through with their type.
func TestFilesFS_SkipsSpecialFiles(t *testing.T) {
	fsys := fstest.MapFS{
		"f.txt": {Data: []byte("x")},
		"link":  {Data: []byte("f.txt"), Mode: fs.ModeSymlink},
		"pipe":  {Mode: fs.ModeNamedPipe},
		"sock":  {Mode: fs.ModeSocket},
		"dev":   {Mode: fs.ModeDevice},
	}
	var files []string
	for p, err := range New().FilesFS(fsys, ".") {
		if err != nil {
			t.Fatalf("FilesFS: %v", err)
		}
		files = append(files, p)
	}
	if want := []string{"f.txt", "link"}; !slices.Equal(files, want) {
		t.Errorf("FilesFS = %q, want %q", files, want)
	}

	var entries []string
	for e, err := range New().EntriesFS(fsys, ".", WalkOptions{}) {
		if err != nil {
			t.Fatalf("EntriesFS: %v", err)
		}
		entries = append(entries, e.Path)
	}
	if want := []string{"dev", "f.txt", "link", "pipe", "sock"}; !slices.Equal(entries, want) {
		t.Errorf("EntriesFS = %q, want %q", entries, want)
	}
}

// A nested .gitignore's warnings reach the handler capped per file.
func TestWalkDirFS_WarningsCappedPerFile(t *testing.T) {
	fsys := fstest.MapFS{
		"sub/.gitignore": {Data: []byte(strings.Repeat("!\n", 8))},
		"sub/x":          {},
	}
	var got []ParseWarning
	m := NewWithOptions(MatcherOptions{MaxWarnings: 3, WarningHandler: func(w ParseWarning) { got = append(got, w) }})
	if err := m.WalkDirFS(fsys, ".", func(string, fs.DirEntry, error) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if len(got) != 4 || got[3].Message != warnLimitMessage || got[3].BasePath != "sub" {
		t.Errorf("handler got %+v, want 3 warnings and the drop marker for sub", got)
	}
}
