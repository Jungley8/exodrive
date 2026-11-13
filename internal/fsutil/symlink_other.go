//go:build !unix && !windows

package fsutil

import "errors"

// makeLink reports that this system cannot link a path to another volume.
func makeLink(string, string) error {
	return errors.New("create link: this system has no implementation")
}
