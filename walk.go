package ignore

import (
	"fmt"
	"io/fs"
	"iter"
	pathpkg "path"
	"path/filepath"
	"slices"
	"strings"
)

// walkBackend captures the filesystem-specific operations that differ between
// the OS-backed WalkDir and the fs.FS-backed WalkDirFS: how to walk, how to
// read a file, how to join path elements, and how to compute paths relative
// to root. Both variants funnel through walkInternal with a backend chosen
// at call time.
type walkBackend struct {
	walkDir  func(root string, fn fs.WalkDirFunc) error
	src      patternSource
	joinPath func(elem ...string) string
	relPath  func(root, p string) (string, error)
}

// WalkDir walks the file tree rooted at root, calling fn for each entry that
// is not ignored by the matcher's rules. As it descends, .gitignore files
// found in each directory are auto-loaded with that directory as their scope;
// rules from those discovered files are visible only during this WalkDir call
// — the receiver matcher is NOT mutated and can be safely reused for
// subsequent walks or Match queries.
//
// Behavior:
//   - fn is called with the same arguments as filepath.WalkDir's WalkDirFunc:
//     an OS-native path (slash on Linux/macOS, backslash on Windows), an
//     fs.DirEntry, and a non-nil err if the entry could not be read. Ignored
//     entries are silently skipped — fn is not called for them.
//   - Ignored directories are pruned (their contents are not visited).
//   - Any entry named .git is skipped wherever it is encountered (not just
//     at the repository root), regardless of matcher rules: a directory holds
//     git internals, and a file is a worktree or submodule gitdir pointer.
//     Match itself does NOT treat .git as special — this is a WalkDir-specific
//     behavior. To walk .git anyway, use filepath.WalkDir directly with Match
//     for filtering.
//   - A nested repository (a directory below root with its own .git) is
//     walked like any other directory. WalkRepo, RepoFiles and RepoEntries
//     stop there, as git does.
//   - Symlinks are not followed (filepath.WalkDir Lstat semantics), and a
//     .gitignore that is a symlink is not read, as in git.
//   - A .gitignore that cannot be read (unreadable, a symlink, not a regular
//     file, or larger than 100 MB) gets exactly one call for its directory,
//     fn(dir, d, err), with err wrapping ErrPatternFileSkipped or the read
//     error. Returning nil walks the directory without that file's rules, as
//     git does; returning fs.SkipDir skips the directory instead.
//   - The IgnoreFiles named in MatcherOptions are read in each directory
//     right after its .gitignore, under the same rules. When several of a
//     directory's files can't be read, the one call carries all their
//     errors, and nil walks the directory with the files that could be read.
//   - Rules from a discovered .gitignore apply only inside its directory and
//     are dropped when the walk leaves it, so MaxPatterns counts the
//     receiver's rules plus those of the current entry's ancestors, not the
//     whole tree.
//
// Paths supplied to user fn are OS-native (the same as filepath.WalkDir).
// Internally the matcher receives the slash-normalised path relative to root,
// so root should be the directory the matcher's rules are scoped to. Walking
// a subdirectory of a repository with a LoadRepo matcher matches "sub/x" as
// "x", and .gitignore files above root are not read.
//
// Parse warnings from the nested .gitignore files discovered during the walk
// are delivered only to a configured WarningHandler (MatcherOptions). Because
// discovery happens on a per-walk internal matcher that is discarded when
// WalkDir returns, such warnings do NOT appear in the receiver's Warnings().
// Set a WarningHandler if you need to observe them.
//
// Thread-safe: the receiver matcher may be queried via Match from other
// goroutines while WalkDir runs; concurrent AddPatterns on the receiver
// during a walk is permitted but will NOT affect the in-progress walk
// (the walker uses a snapshot taken at WalkDir entry).
func (m *Matcher) WalkDir(root string, fn fs.WalkDirFunc) error {
	return m.walkInternal(osBackend, root, "", false, fn)
}

// WalkDirFS is the fs.FS-backed counterpart to WalkDir. It walks the tree at
// root inside fsys, applying the same nested .gitignore discovery
// (MatcherOptions.IgnoreFiles included), rule scoping, handling of ignore
// files it cannot read (one call, 100 MB limit) and .git/-pruning behavior.
// Paths supplied to fn use forward slashes (fs.WalkDir convention),
// regardless of host OS.
//
// Use cases: in-memory tests via fstest.MapFS, embedded filesystems via
// embed.FS, in-process indexers backed by a custom fs.FS, and WASM contexts
// without OS path semantics.
//
// Like WalkDir, the receiver matcher is NOT mutated; discovered rules live
// only for the duration of the call. A .gitignore that fsys reports as a
// symlink (through fs.ReadLinkFS, as fstest.MapFS does) is not read; other
// fs.FS implementations have no symlinks to report.
//
// As with WalkDir, parse warnings from discovered nested .gitignore files
// reach only a configured WarningHandler, not the receiver's Warnings().
//
// Thread-safe: see WalkDir's concurrency notes.
func (m *Matcher) WalkDirFS(fsys fs.FS, root string, fn fs.WalkDirFunc) error {
	return m.walkInternal(fsBackend(fsys), root, "", false, fn)
}

// walkInternal is the shared engine behind WalkDir, WalkDirFS and Entries.
// A non-empty start (already a valid WalkOptions.Start for Entries) walks
// that directory below root after loading the rules above it. stopAtRepos
// makes it list a nested repository as git does, for the repo walkers.
func (m *Matcher) walkInternal(b walkBackend, root, start string, stopAtRepos bool, fn fs.WalkDirFunc) error {
	// Snapshot rules and opts under the read lock so the walker is unaffected
	// by concurrent AddPatterns calls on the receiver.
	m.mu.RLock()
	child := &Matcher{
		opts:      m.opts,
		rules:     slices.Clone(m.rules),
		overrides: m.overrides,
	}
	m.mu.RUnlock()

	// One frame per directory whose .gitignore was loaded, holding the rule
	// count from before it. Leaving the directory truncates back, so only the
	// ancestor chain's rules are live: matching cost stays flat across
	// siblings, and MaxPatterns counts the chain, not the whole tree.
	type frame struct {
		dir   string // rel of the directory, "" for the root
		rules int
	}
	var frames []frame
	// preloading is set while the directories above start are read: they
	// load their rules but reach fn only with an error.
	preloading := false

	visit := func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return fn(path, d, err)
		}

		// rel uses forward slashes, as required for matching against rules.
		rel, relErr := b.relPath(root, path)
		if relErr != nil {
			return fn(path, d, relErr)
		}

		// The walk is depth-first, so a directory that is not an ancestor of
		// this entry has been left for good.
		for len(frames) > 0 {
			top := frames[len(frames)-1]
			if top.dir == "" || strings.HasPrefix(rel, top.dir+"/") {
				break
			}
			child.rules = child.rules[:top.rules]
			frames = frames[:len(frames)-1]
		}

		// Always skip .git (regardless of matcher state): a directory holds git
		// internals, and a file is the gitdir pointer of a worktree or
		// submodule, which git never lists. The root itself (rel == ".") is
		// never skipped even if it happens to be named ".git".
		if rel != "." && d.Name() == ".git" {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}

		if d.IsDir() {

			// Prune ignored directories. The root is always kept.
			if rel != "." && child.match(rel, true, false).Ignored {
				return fs.SkipDir
			}

			// Like git, list a nested repository but not its contents. Above
			// Start it ends the walk: git lists nothing inside one.
			if stopAtRepos && rel != "." && isNestedRepo(path) {
				if preloading {
					return fs.SkipAll
				}
				if err := fn(path, d, nil); err != nil {
					return err
				}
				return fs.SkipDir
			}

			// Load this directory's ignore files into the per-walk child
			// matcher. Most directories have none, which is not an error.
			files, readErr := readIgnoreFiles(b.src, b.joinPath, path, child.opts.IgnoreFiles, nil)
			if len(files) > 0 {
				basePath := rel
				if basePath == "." {
					basePath = ""
				}
				frames = append(frames, frame{dir: basePath, rules: len(child.rules)})
				for _, f := range files {
					child.addPatternsFromSource(basePath, f.content, f.path)
				}
			}
			if readErr != nil {
				// The only callback for this directory: nil walks it with the
				// files that could be read, as git does, and SkipDir prunes it.
				return fn(path, d, readErr)
			}

			if preloading {
				return nil
			}
			return fn(path, d, nil)
		}

		// File: skip silently if ignored, otherwise hand to caller.
		if child.match(rel, false, false).Ignored {
			return nil
		}
		return fn(path, d, nil)
	}

	if start == "" {
		return b.walkDir(root, visit)
	}
	if err := checkStart(start); err != nil {
		_ = fn("", nil, err)
		return nil
	}

	// Read root and each directory above start as the walk would on its way
	// down. A missing step, a symlink (not followed, as in the walk) or a
	// file ends the walk with an error; an ignored directory, or SkipDir or
	// SkipAll from fn on a read error, ends it quietly.
	preloading = true
	dirs := startDirs(b, root, start)
	last := len(dirs) - 1
	for i, dir := range dirs {
		fi, statErr := b.src.stat(dir, false)
		switch {
		case statErr != nil:
			_ = fn(dir, nil, statErr)
			return nil
		case i > 0 && fi.Mode()&fs.ModeSymlink != 0:
			_ = fn(dir, nil, fmt.Errorf("walk start: %s is a symlink", dir))
			return nil
		case i < last && !fi.IsDir():
			_ = fn(dir, nil, fmt.Errorf("walk start: %s is not a directory", dir))
			return nil
		}
		if i < last {
			if r := visit(dir, fs.FileInfoToDirEntry(fi), nil); r != nil {
				return nil
			}
		}
	}
	preloading = false
	return b.walkDir(dirs[last], visit)
}

// startDirs returns root and each directory down to start, joined as the
// walk joins them; the last is the directory walked.
func startDirs(b walkBackend, root, start string) []string {
	dirs := []string{root}
	if start == "" {
		return dirs
	}
	for seg := range strings.SplitSeq(start, "/") {
		dirs = append(dirs, b.joinPath(dirs[len(dirs)-1], seg))
	}
	return dirs
}

// checkStart rejects a WalkOptions.Start that is not a plain relative slash
// path below root.
func checkStart(start string) error {
	if strings.HasPrefix(start, "/") || strings.Contains(start, `\`) || filepath.VolumeName(filepath.FromSlash(start)) != "" {
		return fmt.Errorf("walk start %q: not a relative slash path", start)
	}
	for seg := range strings.SplitSeq(start, "/") {
		if seg == "" || seg == "." || seg == ".." {
			return fmt.Errorf("walk start %q: empty, \".\" or \"..\" segment", start)
		}
	}
	return nil
}

// osBackend is the walkBackend backed by the OS filesystem.
var osBackend = walkBackend{
	walkDir:  filepath.WalkDir,
	src:      osSource{},
	joinPath: filepath.Join,
	relPath: func(root, p string) (string, error) {
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return "", err
		}
		return filepath.ToSlash(rel), nil
	},
}

// fsBackend builds a walkBackend over the given fs.FS. fs.WalkDir paths are
// already forward-slash, so relPath is a simple prefix-strip.
func fsBackend(fsys fs.FS) walkBackend {
	return walkBackend{
		walkDir:  func(root string, fn fs.WalkDirFunc) error { return fs.WalkDir(fsys, root, fn) },
		src:      fsSource{fsys},
		joinPath: pathpkg.Join,
		relPath: func(root, p string) (string, error) {
			if p == root {
				return ".", nil
			}
			if root == "" || root == "." {
				return p, nil
			}
			prefix := root + "/"
			if !strings.HasPrefix(p, prefix) {
				return "", fmt.Errorf("walk path %q is not under root %q", p, root)
			}
			return p[len(prefix):], nil
		},
	}
}

// WalkRepo is a convenience that combines LoadRepo and WalkDir: it loads
// core.excludesFile and info/exclude as LoadRepo does, then walks root.
// The walker reads root's .gitignore and IgnoreFiles itself, once, so a bad
// one is reported through fn like any nested ignore file instead of failing
// the call. A failure loading the other two sources is returned before the
// walk.
//
// Like git ls-files, WalkRepo does not go inside a nested repository: a
// directory below root whose .git is a git directory, a "gitdir:" file
// pointing to one, or a .git file that can't be read. fn is called for the
// directory, which is then skipped and its ignore files are not read. An
// empty or broken .git does not count.
//
// Use this when you want the standard "walk a working tree, skipping
// ignored files" behavior without managing the matcher yourself.
func WalkRepo(root string, opts MatcherOptions, fn fs.WalkDirFunc) error {
	m, err := loadRepoConfig(root, opts)
	if err != nil {
		return err
	}
	return m.walkInternal(osBackend, root, "", true, fn)
}

// Files returns a range-over-func iterator that yields the OS-native path of
// every non-ignored, non-directory entry under root (regular files and
// symlinks, which are not followed), in the same lexical order
// filepath.WalkDir uses. Directories are not yielded; use Entries or WalkDir
// to observe them.
//
// Errors encountered during traversal are yielded as ("", err) and the walk
// continues with the next entry; return or break out of the range loop to
// stop at the first error. Breaking out stops iteration cleanly via
// fs.SkipAll.
//
// Files inherits WalkDir's behavior: nested .gitignore files, and the
// IgnoreFiles named in MatcherOptions, are discovered as descent happens,
// ignored directories are pruned, .git/ is always pruned, and the receiver
// matcher is NOT mutated.
//
// Usage:
//
//	for path, err := range m.Files(root) {
//	    if err != nil { return err }
//	    process(path)
//	}
//
// Thread-safe: see WalkDir's concurrency notes.
func (m *Matcher) Files(root string) iter.Seq2[string, error] {
	return filesFromWalker(func(fn fs.WalkDirFunc) error {
		return m.WalkDir(root, fn)
	})
}

// FilesFS is the fs.FS-backed counterpart to Files. It walks fsys rooted at
// root and yields the forward-slash path of every non-ignored file (no
// directories), suitable for in-memory tests with fstest.MapFS, embed.FS
// content, or any custom fs.FS.
//
// Same nested-discovery, pruning, error-yielding, and break-via-fs.SkipAll
// semantics as Files; only the filesystem backend differs.
//
// Usage:
//
//	for path, err := range m.FilesFS(fsys, ".") {
//	    if err != nil { return err }
//	    process(path)
//	}
func (m *Matcher) FilesFS(fsys fs.FS, root string) iter.Seq2[string, error] {
	return filesFromWalker(func(fn fs.WalkDirFunc) error {
		return m.WalkDirFS(fsys, root, fn)
	})
}

// filesFromWalker is the shared body of Files and FilesFS: it adapts an
// arbitrary fs.WalkDirFunc-driven walker into a files-only iter.Seq2.
func filesFromWalker(walker func(fs.WalkDirFunc) error) iter.Seq2[string, error] {
	return func(yield func(string, error) bool) {
		err := walker(func(path string, d fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				if !yield("", walkErr) {
					return fs.SkipAll
				}
				return nil
			}
			if d.IsDir() {
				return nil
			}
			if !yield(path, nil) {
				return fs.SkipAll
			}
			return nil
		})
		// fs.SkipAll is swallowed by the walker (returns nil), so any non-nil
		// err here is a real traversal failure the callback never surfaced.
		// Yield it as a best-effort tail signal.
		if err != nil {
			yield("", err)
		}
	}
}

// RepoFiles is a convenience that combines LoadRepo and Files, with the root
// .gitignore handled as in WalkRepo: a bad one is yielded as ("", err) and
// iteration continues. A failure loading core.excludesFile or info/exclude is
// yielded once and ends the iteration.
//
// Files inside a nested repository are not yielded; see WalkRepo.
//
// Use it for the standard one-line "iterate the non-ignored files in this
// repo" pattern:
//
//	for path, err := range ignore.RepoFiles(".", ignore.MatcherOptions{}) {
//	    if err != nil { return err }
//	    process(path)
//	}
func RepoFiles(root string, opts MatcherOptions) iter.Seq2[string, error] {
	return func(yield func(string, error) bool) {
		m, err := loadRepoConfig(root, opts)
		if err != nil {
			yield("", err)
			return
		}
		files := filesFromWalker(func(fn fs.WalkDirFunc) error {
			return m.walkInternal(osBackend, root, "", true, fn)
		})
		for path, walkErr := range files {
			if !yield(path, walkErr) {
				return
			}
		}
	}
}

// WalkOptions configures Entries, EntriesFS and RepoEntries. The zero value
// walks the whole tree.
type WalkOptions struct {
	// Start is a slash path under root to walk instead of root, such as
	// "src" or "cmd/tool". The ignore files of root and of every directory
	// down to Start are read first and paths are matched relative to root,
	// so the result is the part of the whole walk under Start. A Start that
	// is ignored, or below an ignored directory, yields nothing; a file
	// yields itself unless ignored. Start must have no empty, "." or ".."
	// segment and no backslash; root and the directories on the way must be
	// real directories, and Start itself must not be a symlink. Otherwise one
	// error is yielded and the iteration ends. "" walks root.
	Start string
}

// Entry is one non-ignored file or directory that Entries yields.
type Entry struct {
	fs.DirEntry
	// Path is the entry's path as WalkDir passes it: OS-native for Entries
	// and RepoEntries, slash-separated for EntriesFS.
	Path string

	st  *entryState
	seq uint64
}

// entryState is shared by the entries of one walk; seq tells which Entry is
// being yielded.
type entryState struct {
	seq  uint64
	skip bool
}

// SkipDir, called before the loop body returns, prunes a directory's
// contents. On the error entry of a directory above WalkOptions.Start it
// ends the iteration instead. It has no effect on a file or after the loop
// body has returned.
func (e Entry) SkipDir() {
	if e.st != nil && e.st.seq == e.seq && e.DirEntry != nil && e.IsDir() {
		e.st.skip = true
	}
}

// Entries returns an iterator over every non-ignored file and directory
// under root, in the lexical order filepath.WalkDir uses, with WalkDir's
// rules: nested ignore files scoped to their directory, ignored directories
// pruned, .git skipped, symlinks not followed, the receiver not mutated.
// The directory walked (root, or WalkOptions.Start) is not yielded. Call
// Entry.SkipDir to prune a directory.
//
// Errors are yielded with the entry they concern and the walk goes on. A
// directory whose ignore files can't be read is yielded once, with the
// error, and then walked with the files that could be read unless SkipDir
// is called; a directory that can't be listed is yielded a second time with
// the error. An error the walk returns at the end is yielded with an empty
// Entry. Breaking out of the loop stops the walk.
//
//	for e, err := range m.Entries(root, ignore.WalkOptions{}) {
//	    if err != nil { return err }
//	    if e.IsDir() && strings.HasPrefix(e.Name(), ".") {
//	        e.SkipDir()
//	    }
//	}
//
// Thread-safe: see WalkDir's concurrency notes.
func (m *Matcher) Entries(root string, wo WalkOptions) iter.Seq2[Entry, error] {
	return m.entries(osBackend, root, wo, false)
}

// EntriesFS is the fs.FS-backed counterpart to Entries: the same walk over
// fsys, with forward-slash paths.
func (m *Matcher) EntriesFS(fsys fs.FS, root string, wo WalkOptions) iter.Seq2[Entry, error] {
	return m.entries(fsBackend(fsys), root, wo, false)
}

// RepoEntries combines LoadRepo's sources and Entries, as WalkRepo does for
// WalkDir: core.excludesFile and info/exclude are loaded first, and a failure
// loading them is yielded once and ends the iteration; the walker reads the
// root's ignore files itself. A nested repository is yielded as a directory
// with nothing under it, as in WalkRepo, and a WalkOptions.Start at or below
// one yields nothing.
func RepoEntries(root string, opts MatcherOptions, wo WalkOptions) iter.Seq2[Entry, error] {
	return func(yield func(Entry, error) bool) {
		m, err := loadRepoConfig(root, opts)
		if err != nil {
			yield(Entry{}, err)
			return
		}
		for e, walkErr := range m.entries(osBackend, root, wo, true) {
			if !yield(e, walkErr) {
				return
			}
		}
	}
}

func (m *Matcher) entries(b walkBackend, root string, wo WalkOptions, stopAtRepos bool) iter.Seq2[Entry, error] {
	dirs := startDirs(b, root, wo.Start)
	return entriesFromWalker(dirs[len(dirs)-1], func(fn fs.WalkDirFunc) error {
		return m.walkInternal(b, root, wo.Start, stopAtRepos, fn)
	})
}

// entriesFromWalker adapts a WalkDirFunc-driven walk to an Entry iterator.
// The walk root is left out when it is a directory, as Entries documents.
func entriesFromWalker(walkRoot string, walker func(fs.WalkDirFunc) error) iter.Seq2[Entry, error] {
	return func(yield func(Entry, error) bool) {
		st := &entryState{}
		err := walker(func(p string, d fs.DirEntry, walkErr error) error {
			if walkErr == nil && p == walkRoot && d.IsDir() {
				return nil
			}
			st.seq++
			st.skip = false
			if !yield(Entry{DirEntry: d, Path: p, st: st, seq: st.seq}, walkErr) {
				return fs.SkipAll
			}
			if st.skip {
				return fs.SkipDir
			}
			return nil
		})
		// As in Files: SkipAll is swallowed by the walk, so this is a failure
		// the callback never saw.
		if err != nil {
			yield(Entry{}, err)
		}
	}
}
