package ignore

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	pathpkg "path"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
)

// Tree checks single paths in a work tree against every .gitignore that
// applies to them, as git check-ignore does: the rules of the Matcher it was
// made from, then each directory's .gitignore from the root down to the
// path's directory, each scoped to its directory. Use it to check, filter or
// explain paths one at a time; use WalkDir to visit a whole tree.
//
// Each directory's .gitignore is read the first time a check needs it, and
// kept for the Tree's lifetime, a read failure included: later edits are not
// seen, so make a new Tree to see them. A .gitignore inside an ignored
// directory is never read, as in git. Reads follow WalkDir's rules: a
// .gitignore that is a symlink, is not a regular file, or is larger than 100
// MB is not read.
//
// A Tree is safe for concurrent use.
type Tree struct {
	base  []rule
	opts  MatcherOptions
	root  string
	src   patternSource
	join  func(elem ...string) string
	lstat func(name string) (fs.FileInfo, error)

	mu   sync.RWMutex
	dirs map[string]*dirRules
}

// dirRules holds one directory's .gitignore, read once.
type dirRules struct {
	once  sync.Once
	rules []rule
	chain int // rule count of the snapshot, the ancestors' files and this one
	err   error
}

// Tree returns a Tree for the work tree at root on the OS filesystem. Paths
// passed to Check are relative to root, as in Match, and MatchResult.Source
// names .gitignore files by OS path, as WalkDir does.
//
// The Tree takes a snapshot of m's rules and options: later changes to m do
// not affect it, and m is never changed. Like WalkDir, the Tree reads root's
// .gitignore itself.
func (m *Matcher) Tree(root string) *Tree {
	return m.newTree(root, osSource{}, filepath.Join, os.Lstat)
}

// TreeFS returns a Tree for the work tree at root inside fsys. Paths passed
// to Check use forward slashes, and MatchResult.Source names .gitignore files
// by their fsys path, as WalkDirFS does.
//
// The Tree takes a snapshot of m's rules and options: later changes to m do
// not affect it, and m is never changed. Like WalkDirFS, the Tree reads
// root's .gitignore itself.
func (m *Matcher) TreeFS(fsys fs.FS, root string) *Tree {
	return m.newTree(root, fsSource{fsys}, pathpkg.Join, func(name string) (fs.FileInfo, error) {
		return fs.Lstat(fsys, name)
	})
}

func (m *Matcher) newTree(root string, src patternSource, join func(...string) string, lstat func(string) (fs.FileInfo, error)) *Tree {
	m.mu.RLock()
	base := m.rules[:len(m.rules):len(m.rules)]
	m.mu.RUnlock()
	return &Tree{
		base:  base,
		opts:  m.opts,
		root:  root,
		src:   src,
		join:  join,
		lstat: lstat,
		dirs:  map[string]*dirRules{},
	}
}

// Check reports whether path is ignored, with the rule that decided, as
// CheckIsDir does. Whether path is a directory is decided as git does: a
// trailing slash (or backslash on Windows) means a directory; otherwise path
// is a directory only if lstat says so, so a symlink to a directory, a path
// that does not exist, and one that can't be stat'ed count as files.
func (t *Tree) Check(path string) (MatchResult, error) {
	isDir := strings.HasSuffix(path, "/") || runtime.GOOS == "windows" && strings.HasSuffix(path, `\`)
	if !isDir {
		if p := normalizePath(path); p != "" {
			fi, err := t.lstat(t.join(t.root, p))
			isDir = err == nil && fi.IsDir()
		}
	}
	return t.CheckIsDir(path, isDir)
}

// CheckIsDir reports whether path, relative to the Tree's root, is ignored,
// with the rule that decided, as Matcher.MatchWithReason does. isDir must be
// accurate, as in Match.
//
// path is normalized as in Match. One that is empty, resolves outside the
// root, or is deeper than MaxPathDepth matches nothing.
//
// As in git, a path inside an ignored directory is ignored, and the result
// names the rule that ignored the outermost such directory.
//
// A .gitignore on the way to path that can't be read does not stop the
// check: the result is git's answer without that file, and the error,
// wrapping ErrPatternFileSkipped or the read error, comes with it on every
// check that consults that directory. Several are combined with errors.Join.
func (t *Tree) CheckIsDir(path string, isDir bool) (MatchResult, error) {
	var segBuf [32]string
	disk, match, segs, ok := preparePath(path, t.opts.CaseInsensitive, segBuf[:0])
	if !ok {
		return MatchResult{}, nil
	}

	var groupBuf [34][]rule
	groups := append(groupBuf[:0], t.base)
	var errs []error
	d := t.load("", len(t.base))
	groups = append(groups, d.rules)
	if d.err != nil {
		errs = append(errs, d.err)
	}

	// disk and match have their slashes at the same segment boundaries, so
	// walk both: disk names the directory to read, match is what rules see.
	diskStart, matchStart := 0, 0
	if disk[0] == '/' {
		diskStart, matchStart = 1, 1
	}
	di, mi := diskStart, matchStart
	for k := 1; k < len(segs); k++ {
		di += strings.IndexByte(disk[di:], '/')
		mi += strings.IndexByte(match[mi:], '/')
		if res := evaluateGroups(groups, match[matchStart:mi], segs[:k], true); res.Matched && res.Ignored {
			return res, errors.Join(errs...)
		}
		d = t.load(disk[diskStart:di], d.chain)
		groups = append(groups, d.rules)
		if d.err != nil {
			errs = append(errs, d.err)
		}
		di++
		mi++
	}
	return evaluateGroups(groups, match, segs, isDir), errors.Join(errs...)
}

// evaluateGroups is evaluateRules over several rule lists in order, last
// match winning across all of them.
func evaluateGroups(groups [][]rule, path string, segs []string, isDir bool) MatchResult {
	var res MatchResult
	for _, g := range groups {
		if r := evaluateRules(g, path, segs, isDir); r.Matched {
			res = r
		}
	}
	return res
}

// load returns dir's entry ("" is the root), reading its .gitignore on first
// use. chain is the rule count of the snapshot and dir's ancestors' files.
// Warnings go to the handler after the entry is ready, so the handler may
// call back into the Tree.
func (t *Tree) load(dir string, chain int) *dirRules {
	t.mu.RLock()
	d, ok := t.dirs[dir]
	t.mu.RUnlock()
	if !ok {
		t.mu.Lock()
		if d, ok = t.dirs[dir]; !ok {
			d = &dirRules{}
			t.dirs[dir] = d
		}
		t.mu.Unlock()
	}
	var warnings []ParseWarning
	d.once.Do(func() { warnings = t.read(d, dir, chain) })
	if h := t.opts.WarningHandler; h != nil {
		for _, w := range warnings {
			h(w)
		}
	}
	return d
}

// read fills d from dir's .gitignore and returns the parse warnings when a
// handler wants them.
func (t *Tree) read(d *dirRules, dir string, chain int) []ParseWarning {
	d.chain = chain
	name := t.join(t.root, dir, ".gitignore")
	content, err := readPatternFile(t.src, name, false, maxPatternFileSize)
	if err != nil {
		// ENOTDIR: a path below a file, as in Check("file.txt/x"), asks
		// for file.txt/.gitignore, which can't exist.
		if !errors.Is(err, fs.ErrNotExist) && !errors.Is(err, syscall.ENOTDIR) {
			d.err = fmt.Errorf("reading %s: %w", name, err)
		}
		return nil
	}

	var warnings []ParseWarning
	// A scratch Matcher parses as the walker does: the same limits,
	// CaseInsensitive scoping and warnings.
	s := &Matcher{opts: t.opts}
	s.opts.WarningHandler = nil
	if t.opts.WarningHandler != nil {
		s.opts.WarningHandler = func(w ParseWarning) { warnings = append(warnings, w) }
	}
	if maxPatterns := s.effectiveMaxPatterns(); maxPatterns >= 0 {
		remaining := maxPatterns - chain
		if remaining <= 0 {
			return append(warnings, ParseWarning{
				Message:  "maximum pattern count reached, new patterns skipped",
				BasePath: dir,
			})
		}
		s.opts.MaxPatterns = remaining
	}
	s.addPatternsFromSource(dir, content, name)
	d.rules = s.rules
	d.chain += len(d.rules)
	return warnings
}
