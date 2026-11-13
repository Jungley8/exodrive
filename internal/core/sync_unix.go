//go:build unix

package core

import "os"

// syncDir flushes a directory, so the rename survives a power loss.
func syncDir(dir *os.File) error {
	return dir.Sync()
}
