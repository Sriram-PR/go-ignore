package ignore

import (
	"errors"
	"fmt"
	"io/fs"
	"iter"
	"os"
	pathpkg "path"
	"path/filepath"
	"strings"
)

// walkBackend captures the filesystem-specific operations that differ between
// the OS-backed WalkDir and the fs.FS-backed WalkDirFS: how to walk, how to
// read a file, how to join path elements, and how to compute paths relative
// to root. Both variants funnel through walkInternal with a backend chosen
// at call time.
type walkBackend struct {
	walkDir  func(root string, fn fs.WalkDirFunc) error
	readFile func(path string) ([]byte, error)
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
//   - The .git directory at the repository root is always pruned, regardless
//     of matcher rules, to avoid walking git internals. Match itself does NOT
//     treat .git as special — this prune is a WalkDir-specific behavior. To
//     walk .git anyway, use filepath.WalkDir directly with Match for filtering.
//   - Symlinks are not followed (filepath.WalkDir Lstat semantics).
//
// Paths supplied to user fn are OS-native (the same as filepath.WalkDir).
// Internally the matcher receives the slash-normalised relative path.
//
// Thread-safe: the receiver matcher may be queried via Match from other
// goroutines while WalkDir runs; concurrent AddPatterns on the receiver
// during a walk is permitted but will NOT affect the in-progress walk
// (the walker uses a snapshot taken at WalkDir entry).
func (m *Matcher) WalkDir(root string, fn fs.WalkDirFunc) error {
	return m.walkInternal(osBackend, root, fn)
}

// WalkDirFS is the fs.FS-backed counterpart to WalkDir. It walks the tree at
// root inside fsys, applying the same nested .gitignore discovery and
// .git/-pruning behavior. Paths supplied to fn use forward slashes (fs.WalkDir
// convention), regardless of host OS.
//
// Use cases: in-memory tests via fstest.MapFS, embedded filesystems via
// embed.FS, in-process indexers backed by a custom fs.FS, and WASM contexts
// without OS path semantics.
//
// Like WalkDir, the receiver matcher is NOT mutated; discovered rules live
// only for the duration of the call. The .gitignore lookup uses fs.ReadFile
// — symlink handling is whatever fsys provides (most fs.FS implementations,
// including fstest.MapFS and embed.FS, have no concept of symlinks).
//
// Thread-safe: see WalkDir's concurrency notes.
func (m *Matcher) WalkDirFS(fsys fs.FS, root string, fn fs.WalkDirFunc) error {
	return m.walkInternal(fsBackend(fsys), root, fn)
}

// walkInternal is the shared engine behind WalkDir and WalkDirFS.
func (m *Matcher) walkInternal(b walkBackend, root string, fn fs.WalkDirFunc) error {
	// Snapshot rules and opts under the read lock so the walker is unaffected
	// by concurrent AddPatterns calls on the receiver.
	m.mu.RLock()
	child := &Matcher{
		opts:  m.opts,
		rules: append([]rule(nil), m.rules...),
	}
	m.mu.RUnlock()

	return b.walkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return fn(path, d, err)
		}

		// Compute path relative to root using forward slashes for matching.
		rel, relErr := b.relPath(root, path)
		if relErr != nil {
			return fn(path, d, relErr)
		}

		if d.IsDir() {
			// Always prune .git (regardless of matcher state) so walks of real
			// repos don't descend into git internals. The root itself (rel
			// == ".") is never pruned even if it happens to be named ".git".
			if rel != "." && d.Name() == ".git" {
				return fs.SkipDir
			}

			// Prune ignored directories. The root is always kept.
			if rel != "." && child.Match(rel, true) {
				return fs.SkipDir
			}

			// Discover a .gitignore in this directory and load it into the
			// per-walk child matcher. ReadFile returns a not-exist error for
			// directories without a .gitignore — that's the common case and
			// silently ignored. Other read errors flow through fn.
			gitignorePath := b.joinPath(path, ".gitignore")
			content, readErr := b.readFile(gitignorePath)
			switch {
			case readErr == nil:
				basePath := rel
				if basePath == "." {
					basePath = ""
				}
				child.addPatternsFromSource(basePath, content, gitignorePath)
			case !errors.Is(readErr, fs.ErrNotExist):
				if cbErr := fn(path, d, fmt.Errorf("reading %s: %w", gitignorePath, readErr)); cbErr != nil {
					return cbErr
				}
			}

			return fn(path, d, nil)
		}

		// File: skip silently if ignored, otherwise hand to caller.
		if child.Match(rel, false) {
			return nil
		}
		return fn(path, d, nil)
	})
}

// osBackend is the walkBackend backed by the OS filesystem.
var osBackend = walkBackend{
	walkDir:  filepath.WalkDir,
	readFile: os.ReadFile,
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
		readFile: func(p string) ([]byte, error) { return fs.ReadFile(fsys, p) },
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

// WalkRepo is a convenience that combines LoadRepo and WalkDir. It is
// equivalent to:
//
//	m, err := ignore.LoadRepo(root, opts)
//	if err != nil { return err }
//	return m.WalkDir(root, fn)
//
// Use this when you want the standard "walk a working tree, skipping
// ignored files" behavior without managing the matcher yourself.
func WalkRepo(root string, opts MatcherOptions, fn fs.WalkDirFunc) error {
	m, err := LoadRepo(root, opts)
	if err != nil {
		return err
	}
	return m.WalkDir(root, fn)
}

// Files returns a range-over-func iterator that yields the OS-native path of
// every non-ignored regular file under root, in the same lexical order
// filepath.WalkDir uses. Directories are not yielded — use WalkDir if you
// need to observe them.
//
// Errors encountered during traversal are yielded as ("", err); after a
// non-nil error is yielded, iteration ends. Breaking out of the range loop
// stops iteration cleanly via fs.SkipAll.
//
// Files inherits WalkDir's behavior: nested .gitignore files are discovered
// as descent happens, ignored directories are pruned, .git/ is always pruned,
// and the receiver matcher is NOT mutated.
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

// RepoFiles is a convenience that combines LoadRepo and Files. It is
// equivalent to:
//
//	m, err := ignore.LoadRepo(root, opts)
//	if err != nil { /* yield ("", err) and stop */ }
//	for path, walkErr := range m.Files(root) { yield(path, walkErr) }
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
		m, err := LoadRepo(root, opts)
		if err != nil {
			yield("", err)
			return
		}
		for path, walkErr := range m.Files(root) {
			if !yield(path, walkErr) {
				return
			}
		}
	}
}
