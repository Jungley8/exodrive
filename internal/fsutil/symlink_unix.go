//go:build unix

package fsutil

import "os"

// makeLink creates a symbolic link that points at target.
func makeLink(target, linkPath string) error {
	return os.Symlink(target, linkPath)
}
