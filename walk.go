package ignore

import (
	"io/fs"
	"os"
	"path/filepath"
)

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
	// Snapshot rules and opts under the read lock so the walker is unaffected
	// by concurrent AddPatterns calls on the receiver.
	m.mu.RLock()
	child := &Matcher{
		opts:  m.opts,
		rules: append([]rule(nil), m.rules...),
	}
	m.mu.RUnlock()

	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return fn(path, d, err)
		}

		// Compute path relative to root using forward slashes for matching.
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return fn(path, d, relErr)
		}
		rel = filepath.ToSlash(rel)

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
			// per-walk child matcher. Errors reading the file are reported
			// through fn so callers can decide whether to abort.
			gitignorePath := filepath.Join(path, ".gitignore")
			if info, statErr := os.Lstat(gitignorePath); statErr == nil && !info.IsDir() {
				basePath := rel
				if basePath == "." {
					basePath = ""
				}
				if loadErr := child.AddPatternsFromFile(basePath, gitignorePath); loadErr != nil {
					if cbErr := fn(path, d, loadErr); cbErr != nil {
						return cbErr
					}
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
