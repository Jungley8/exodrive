//go:build !unix

package core

import "os"

// syncDir does nothing, because Windows cannot flush a directory handle.
// The ledger file itself was flushed before the rename, so the data is on
// disk; only the directory entry waits for the system to write it.
func syncDir(*os.File) error {
	return nil
}
