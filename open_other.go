//go:build !unix

package ignore

import "os"

// openPatternFile relies on readPatternFile's checks, before the open and on
// the open handle, to refuse symlinks and non-regular files.
func openPatternFile(name string, _ bool) (*os.File, error) {
	return os.Open(name)
}
