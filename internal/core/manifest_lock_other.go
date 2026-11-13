//go:build !unix && !windows

package core

// lockManifest is a no-op on systems with no file lock. Exodrive refuses the
// migration on them elsewhere; this keeps the package buildable.
func lockManifest() (func(), error) { return lockPath(GetManifestPath()) }

// lockPath is a no-op on systems with no file lock.
func lockPath(string) (func(), error) { return func() {}, nil }
