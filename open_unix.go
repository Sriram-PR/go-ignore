//go:build unix

package ignore

import (
	"os"
	"syscall"
)

// openPatternFile opens with O_NONBLOCK so a FIFO swapped in after the type
// check cannot block the open, and with O_NOFOLLOW unless follow is set so a
// symlink swapped in is refused.
func openPatternFile(name string, follow bool) (*os.File, error) {
	flags := os.O_RDONLY | syscall.O_NONBLOCK
	if !follow {
		flags |= syscall.O_NOFOLLOW
	}
	return os.OpenFile(name, flags, 0)
}
