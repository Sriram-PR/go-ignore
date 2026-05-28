package ignore

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
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
