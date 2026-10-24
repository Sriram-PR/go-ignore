package ignore_test

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing/fstest"

	ignore "github.com/Sriram-PR/go-ignore"
)

func ExampleNew() {
	m := ignore.New()
	m.AddPatterns("", []byte("*.log\nbuild/\n!important.log\n"))

	fmt.Println(m.Match("debug.log", false))
	fmt.Println(m.Match("src/main.go", false))
	fmt.Println(m.Match("important.log", false))
	fmt.Println(m.Match("build/output.js", false))
	// Output:
	// true
	// false
	// false
	// true
}

func ExampleMatcher_MatchWithReason() {
	m := ignore.New()
	m.AddPatterns("", []byte("*.log\n!important.log\n"))

	result := m.MatchWithReason("debug.log", false)
	fmt.Printf("ignored=%v rule=%q\n", result.Ignored, result.Rule)

	result = m.MatchWithReason("important.log", false)
	fmt.Printf("ignored=%v negated=%v rule=%q\n", result.Ignored, result.Negated(), result.Rule)
	// Output:
	// ignored=true rule="*.log"
	// ignored=false negated=true rule="!important.log"
}

func ExampleMatcherOptions_warningHandler() {
	m := ignore.NewWithOptions(ignore.MatcherOptions{
		WarningHandler: func(w ignore.ParseWarning) {
			fmt.Printf("line %d: %s\n", w.Line, w.Message)
		},
	})
	m.AddPatterns("", []byte("*.log\n!\n"))
	// Output:
	// line 2: pattern is empty after processing
}

func ExampleMatcher_AddGlobalPatterns() {
	m := ignore.New()
	if err := m.AddGlobalPatterns(); err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Println("loaded:", m.RuleCount() >= 0)
	// Output:
	// loaded: true
}

func ExampleMatcher_AddExcludePatterns() {
	// Simulate a repo with a .git/info/exclude file.
	gitDir, err := os.MkdirTemp("", "go-ignore-example-*")
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	defer os.RemoveAll(gitDir)

	if err := os.MkdirAll(filepath.Join(gitDir, "info"), 0o755); err != nil {
		fmt.Println("error:", err)
		return
	}
	excludeFile := filepath.Join(gitDir, "info", "exclude")
	if err := os.WriteFile(excludeFile, []byte("*.local\n"), 0o644); err != nil {
		fmt.Println("error:", err)
		return
	}

	m := ignore.New()
	if err := m.AddExcludePatterns(gitDir); err != nil {
		fmt.Println("error:", err)
		return
	}

	fmt.Println(m.Match("config.local", false))
	fmt.Println(m.Match("config.yaml", false))
	// Output:
	// true
	// false
}

func ExampleWalkRepo() {
	// Build a small repo on disk: a .gitignore plus some files, some ignored.
	root, err := os.MkdirTemp("", "go-ignore-walkrepo-*")
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	defer os.RemoveAll(root)

	must := func(rel, content string) {
		full := filepath.Join(root, filepath.FromSlash(rel))
		_ = os.MkdirAll(filepath.Dir(full), 0o755)
		_ = os.WriteFile(full, []byte(content), 0o644)
	}
	must(".gitignore", "*.log\nbuild/\n")
	must("keep.txt", "x")
	must("debug.log", "x")    // ignored
	must("build/out.js", "x") // ignored (parent dir)
	must("src/main.go", "x")

	// WalkRepo loads global + .git/info/exclude + root .gitignore, then walks
	// the tree skipping any path the matcher considers ignored.
	var got []string
	_ = ignore.WalkRepo(root, ignore.MatcherOptions{}, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		got = append(got, filepath.ToSlash(rel))
		return nil
	})
	sort.Strings(got)
	for _, p := range got {
		fmt.Println(p)
	}
	// Output:
	// .gitignore
	// keep.txt
	// src/main.go
}

func ExampleRepoFiles() {
	root, err := os.MkdirTemp("", "go-ignore-repofiles-*")
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	defer os.RemoveAll(root)

	must := func(rel, content string) {
		full := filepath.Join(root, filepath.FromSlash(rel))
		_ = os.MkdirAll(filepath.Dir(full), 0o755)
		_ = os.WriteFile(full, []byte(content), 0o644)
	}
	must(".gitignore", "*.log\n")
	must("keep.txt", "x")
	must("debug.log", "x")

	// RepoFiles is the iterator form of WalkRepo — yields only non-ignored
	// files, suitable for "for path, err := range ..." loops.
	var got []string
	for path, err := range ignore.RepoFiles(root, ignore.MatcherOptions{}) {
		if err != nil {
			fmt.Println("error:", err)
			return
		}
		rel, _ := filepath.Rel(root, path)
		got = append(got, filepath.ToSlash(rel))
	}
	sort.Strings(got)
	for _, p := range got {
		fmt.Println(p)
	}
	// Output:
	// .gitignore
	// keep.txt
}

func ExampleNewWithOptions() {
	m := ignore.NewWithOptions(ignore.MatcherOptions{
		CaseInsensitive: true,
	})
	m.AddPatterns("", []byte("*.LOG\n"))

	fmt.Println(m.Match("debug.log", false))
	fmt.Println(m.Match("DEBUG.LOG", false))
	// Output:
	// true
	// true
}

func ExampleMatcherOptions_overrides() {
	m := ignore.NewWithOptions(ignore.MatcherOptions{
		Overrides: []string{"*.pem", "!build/"},
	})
	m.AddPatterns("", []byte("build/\n!*.pem\n"))
	fmt.Println(m.Match("key.pem", false))
	fmt.Println(m.Match("build/app", false))
	r := m.MatchWithReason("key.pem", false)
	fmt.Println(r.Source, r.Line)
	// Output:
	// true
	// false
	// overrides 1
}

func ExampleMatcherOptions_ignoreFiles() {
	fsys := fstest.MapFS{
		".gitignore":  {Data: []byte("*.log\n")},
		".aiignore":   {Data: []byte("secrets/\n")},
		"app.go":      {},
		"debug.log":   {},
		"secrets/key": {},
	}
	m := ignore.NewWithOptions(ignore.MatcherOptions{IgnoreFiles: []string{".aiignore"}})
	for p, err := range m.FilesFS(fsys, ".") {
		if err != nil {
			fmt.Println("error:", err)
			return
		}
		fmt.Println(p)
	}
	// Output:
	// .aiignore
	// .gitignore
	// app.go
}

func ExampleMatcher_EntriesFS() {
	fsys := fstest.MapFS{
		".gitignore":     {Data: []byte("*.log\n")},
		".venv/lib/x.py": {},
		"main.go":        {},
		"pkg/debug.log":  {},
		"pkg/util.go":    {},
	}
	for e, err := range ignore.New().EntriesFS(fsys, ".", ignore.WalkOptions{}) {
		if err != nil {
			fmt.Println("error:", err)
			return
		}
		if e.IsDir() {
			if strings.HasPrefix(e.Name(), ".") {
				e.SkipDir()
			}
			continue
		}
		fmt.Println(e.Path)
	}
	// Output:
	// .gitignore
	// main.go
	// pkg/util.go
}

func ExampleMatcher_TreeFS() {
	fsys := fstest.MapFS{
		".gitignore":     {Data: []byte("*.log\n")},
		"src/.gitignore": {Data: []byte("*.tmp\n!keep.log\n")},
	}
	tr := ignore.New().TreeFS(fsys, ".")
	for _, p := range []string{"src/cache.tmp", "src/keep.log", "debug.log", "lib/cache.tmp"} {
		r, err := tr.Check(p)
		if err != nil {
			fmt.Println(err)
			continue
		}
		fmt.Printf("%-14s ignored=%-5v %s:%d %s\n", p, r.Ignored, r.Source, r.Line, r.Rule)
	}
	// Output:
	// src/cache.tmp  ignored=true  src/.gitignore:1 *.tmp
	// src/keep.log   ignored=false src/.gitignore:2 !keep.log
	// debug.log      ignored=true  .gitignore:1 *.log
	// lib/cache.tmp  ignored=false :0
}
