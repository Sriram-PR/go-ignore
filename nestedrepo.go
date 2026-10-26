package ignore

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// maxGitFileSize is git's limit for a .git file (read_gitfile_gently).
const maxGitFileSize = 1 << 20

// isNestedRepo reports whether dir is the work tree of another repository,
// as git's is_nonbare_repository_dir decides: dir/.git is a git directory, a
// .git file pointing to one, or a .git file git cannot open or read.
func isNestedRepo(dir string) bool {
	gitPath := filepath.Join(dir, ".git")
	fi, err := os.Stat(gitPath)
	switch {
	case err != nil:
		return false
	case fi.IsDir():
		return isGitDirectory(gitPath)
	case !fi.Mode().IsRegular() || fi.Size() > maxGitFileSize:
		return false
	}
	content, err := readPatternFile(osSource{}, gitPath, true, maxGitFileSize)
	if err != nil {
		return !errors.Is(err, fs.ErrNotExist) && !errors.Is(err, ErrPatternFileSkipped)
	}
	target, ok := strings.CutPrefix(strings.TrimRight(string(content), "\r\n"), "gitdir: ")
	if !ok || target == "" {
		return false
	}
	return isGitDirectory(resolveRelative(dir, target))
}

// isGitDirectory is git's is_git_directory: a valid HEAD, and objects and
// refs in the common directory. Git checks objects and refs with
// access(X_OK); a directory check stands in for it.
func isGitDirectory(gitDir string) bool {
	if !validHeadRef(filepath.Join(gitDir, "HEAD")) {
		return false
	}
	common := gitDir
	if c, err := readPatternFile(osSource{}, filepath.Join(gitDir, "commondir"), true, maxGitFileSize); err == nil && len(c) > 0 {
		dir := string(c)
		if d, ok := strings.CutSuffix(dir, "\n"); ok {
			dir = strings.TrimSuffix(d, "\r")
		}
		common = resolveRelative(gitDir, dir)
	}
	return isDirPath(filepath.Join(common, "objects")) && isDirPath(filepath.Join(common, "refs"))
}

// validHeadRef is git's validate_headref: HEAD is a symlink into refs/, a
// "ref: refs/..." symbolic ref, or starts with a full object id.
func validHeadRef(head string) bool {
	fi, err := os.Lstat(head)
	if err != nil {
		return false
	}
	if fi.Mode()&fs.ModeSymlink != 0 {
		target, linkErr := os.Readlink(head)
		return linkErr == nil && strings.HasPrefix(target, "refs/")
	}
	if !fi.Mode().IsRegular() {
		return false
	}
	f, err := openPatternFile(head, false)
	if err != nil {
		return false
	}
	defer f.Close()
	// Git reads at most 255 bytes of HEAD.
	buf, err := io.ReadAll(io.LimitReader(f, 255))
	if err != nil {
		return false
	}
	s := string(buf)
	if ref, ok := strings.CutPrefix(s, "ref:"); ok {
		return strings.HasPrefix(strings.TrimLeft(ref, " \t\n\r"), "refs/")
	}
	return hexPrefix(s, 40) || hexPrefix(s, 64)
}

// hexPrefix reports whether s starts with n hex digits, SHA-1 (40) or
// SHA-256 (64) object ids.
func hexPrefix(s string, n int) bool {
	if len(s) < n {
		return false
	}
	for i := range n {
		switch c := s[i]; {
		case '0' <= c && c <= '9', 'a' <= c && c <= 'f', 'A' <= c && c <= 'F':
		default:
			return false
		}
	}
	return true
}

func isDirPath(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.IsDir()
}
