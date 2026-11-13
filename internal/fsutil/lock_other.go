//go:build !unix && !windows

package fsutil

import "errors"

// listProcesses reports that this system has no process table reader, so the
// check refuses the migration instead of skipping it.
func listProcesses() ([]processEntry, error) {
	return nil, errors.New("this system has no process table reader")
}
