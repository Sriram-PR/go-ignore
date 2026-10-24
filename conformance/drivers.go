// Package conformance compares go-ignore and other Go gitignore libraries
// with git check-ignore and renders the result for the README. It is a
// separate module so the library's go.mod stays free of the libraries it is
// compared with.
package conformance

import (
	"cmp"
	"errors"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"runtime/debug"
	"slices"
	"strings"

	ignore "github.com/Sriram-PR/go-ignore"
	gitpkgs "github.com/git-pkgs/gitignore"
	"github.com/go-git/go-billy/v5/osfs"
	gogit "github.com/go-git/go-git/v5/plumbing/format/gitignore"
	sabhiram "github.com/sabhiram/go-gitignore"
)

// matchFunc reports whether a slash path relative to the scenario root is
// ignored.
type matchFunc func(path string, isDir bool) bool

// driver adapts one library: load reads the scenario directory with the
// library's documented loader.
type driver struct {
	name   string // column header
	module string // module path, for the version shown
	api    string // what the driver calls, shown in RESULTS.md
	nested bool   // false: scenarios with a non-root .gitignore are n/a
	load   func(dir string) (matchFunc, error)
}

// drivers is in column order.
var drivers = []driver{
	{name: "go-ignore", module: "github.com/Sriram-PR/go-ignore", api: "Matcher.AddPatternsWithSource + MatchWithReason", nested: true, load: loadGoIgnore},
	{name: "git-pkgs", module: "github.com/git-pkgs/gitignore", api: "NewFromDirectory + MatchPath", nested: true, load: loadGitPkgs},
	{name: "go-git v5", module: "github.com/go-git/go-git/v5", api: "ReadPatterns + NewMatcher + Match", nested: true, load: loadGoGit},
	{name: "sabhiram", module: "github.com/sabhiram/go-gitignore", api: "CompileIgnoreFile + MatchesPath (takes no isDir)", nested: false, load: loadSabhiram},
}

// loadGoIgnore adds every .gitignore shallowest first with its directory as
// base, as the walker does and as the parity harness's parityMatcher does.
func loadGoIgnore(dir string) (matchFunc, error) {
	var files []string
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() && d.Name() == ".git" {
			return fs.SkipDir
		}
		if !d.IsDir() && d.Name() == ".gitignore" {
			rel, relErr := filepath.Rel(dir, p)
			if relErr != nil {
				return relErr
			}
			files = append(files, filepath.ToSlash(rel))
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	slices.SortFunc(files, func(a, b string) int {
		if c := cmp.Compare(strings.Count(a, "/"), strings.Count(b, "/")); c != 0 {
			return c
		}
		return cmp.Compare(a, b)
	})
	m := ignore.NewWithOptions(ignore.MatcherOptions{MaxPatterns: -1, WarningHandler: func(ignore.ParseWarning) {}})
	for _, f := range files {
		content, readErr := os.ReadFile(filepath.Join(dir, filepath.FromSlash(f)))
		if readErr != nil {
			return nil, readErr
		}
		base := path.Dir(f)
		if base == "." {
			base = ""
		}
		m.AddPatternsWithSource(base, f, content)
	}
	return func(p string, isDir bool) bool { return m.MatchWithReason(p, isDir).Ignored }, nil
}

func loadGitPkgs(dir string) (matchFunc, error) {
	return gitpkgs.NewFromDirectory(dir).MatchPath, nil
}

func loadGoGit(dir string) (matchFunc, error) {
	ps, err := gogit.ReadPatterns(osfs.New(dir), nil)
	if err != nil {
		return nil, err
	}
	m := gogit.NewMatcher(ps)
	return func(p string, isDir bool) bool { return m.Match(strings.Split(p, "/"), isDir) }, nil
}

// loadSabhiram reads only the root file: the library has no notion of nested
// files. Its matcher takes no isDir and documents no trailing-slash
// convention, so paths are passed as they are.
func loadSabhiram(dir string) (matchFunc, error) {
	gi, err := sabhiram.CompileIgnoreFile(filepath.Join(dir, ".gitignore"))
	if errors.Is(err, fs.ErrNotExist) {
		gi = sabhiram.CompileIgnoreLines()
	} else if err != nil {
		return nil, err
	}
	return func(p string, _ bool) bool { return gi.MatchesPath(p) }, nil
}

// moduleVersion is the version of module that this binary was built with.
func moduleVersion(module string) string {
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return "unknown"
	}
	for _, d := range bi.Deps {
		if d.Path != module {
			continue
		}
		if d.Replace != nil {
			return "(this repo)"
		}
		return d.Version
	}
	return "unknown"
}
