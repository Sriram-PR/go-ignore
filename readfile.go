package ignore

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
)

// ErrPatternFileSkipped reports a pattern file that was deliberately not
// read, because it is:
//   - a symlink where symlinks are not followed (a .gitignore in the
//     working tree, as in git);
//   - not a regular file (a directory, FIFO, device or socket);
//   - larger than 100 MB, git's limit for pattern files.
//
// Errors wrap it with the path and the reason; test with errors.Is.
var ErrPatternFileSkipped = errors.New("pattern file skipped")

// maxPatternFileSize is git's limit for a pattern file (dir.c).
const maxPatternFileSize = 100 << 20

// patternSource is the filesystem a pattern file is read from.
type patternSource interface {
	stat(name string, follow bool) (fs.FileInfo, error)
	open(name string, follow bool) (fs.File, error)
}

type osSource struct{}

func (osSource) stat(name string, follow bool) (fs.FileInfo, error) {
	if follow {
		return os.Stat(name)
	}
	return os.Lstat(name)
}

func (osSource) open(name string, follow bool) (fs.File, error) {
	return openPatternFile(name, follow)
}

type fsSource struct{ fsys fs.FS }

// stat uses fs.Lstat, which falls back to Stat when fsys does not implement
// fs.ReadLinkFS.
func (s fsSource) stat(name string, follow bool) (fs.FileInfo, error) {
	if follow {
		return fs.Stat(s.fsys, name)
	}
	return fs.Lstat(s.fsys, name)
}

func (s fsSource) open(name string, _ bool) (fs.File, error) {
	return s.fsys.Open(name)
}

// readPatternFile reads a pattern file of at most limit bytes. A missing
// file returns an error satisfying errors.Is(err, fs.ErrNotExist); a
// refused one returns an error wrapping ErrPatternFileSkipped.
func readPatternFile(src patternSource, name string, follow bool, limit int64) ([]byte, error) {
	fi, err := src.stat(name, follow)
	if err != nil {
		return nil, err
	}
	if err = checkPatternFile(fi, limit); err != nil {
		return nil, err
	}
	f, err := src.open(name, follow)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	// The file may have been replaced since the stat.
	if fi, err = f.Stat(); err != nil {
		return nil, err
	}
	if err = checkPatternFile(fi, limit); err != nil {
		return nil, err
	}
	return readLimited(f, limit)
}

func checkPatternFile(fi fs.FileInfo, limit int64) error {
	switch {
	case fi.Mode()&fs.ModeSymlink != 0:
		return fmt.Errorf("%w: symlink", ErrPatternFileSkipped)
	case !fi.Mode().IsRegular():
		return fmt.Errorf("%w: not a regular file", ErrPatternFileSkipped)
	case fi.Size() > limit:
		return errTooLarge(limit)
	}
	return nil
}

// readLimited reads all of r, refusing more than limit bytes. It reads one
// byte past the limit, so a file that grew after its size check is caught.
func readLimited(r io.Reader, limit int64) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, errTooLarge(limit)
	}
	return data, nil
}

func errTooLarge(limit int64) error {
	if limit == maxPatternFileSize {
		return fmt.Errorf("%w: larger than 100 MB", ErrPatternFileSkipped)
	}
	return fmt.Errorf("%w: larger than %d bytes", ErrPatternFileSkipped, limit)
}

// ignoreFile is one per-directory ignore file that was read.
type ignoreFile struct {
	path    string
	content []byte
}

// readIgnoreFiles reads dir's .gitignore and then each extra name, in order,
// as readPatternFile does. Missing files, and errors skip accepts, are left
// out silently; the other errors are joined, each naming its file.
func readIgnoreFiles(src patternSource, join func(...string) string, dir string, extra []string, skip func(error) bool) ([]ignoreFile, error) {
	var files []ignoreFile
	var errs []error
	read := func(name string) {
		p := join(dir, name)
		content, err := readPatternFile(src, p, false, maxPatternFileSize)
		switch {
		case err == nil:
			files = append(files, ignoreFile{p, content})
		case !errors.Is(err, fs.ErrNotExist) && (skip == nil || !skip(err)):
			errs = append(errs, fmt.Errorf("reading %s: %w", p, err))
		}
	}
	read(".gitignore")
	for _, name := range extra {
		read(name)
	}
	return files, errors.Join(errs...)
}
