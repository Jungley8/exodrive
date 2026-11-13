//go:build !unix

package core

// lockManifest is a no-op on platforms without flock. Exodrive supports macOS
// and Linux; this keeps the package buildable elsewhere.
func lockManifest() (func(), error) { return lockPath(GetManifestPath()) }

// lockPath is a no-op on platforms without flock.
func lockPath(string) (func(), error) { return func() {}, nil }
