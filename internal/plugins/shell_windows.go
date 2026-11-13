//go:build !unix

package plugins

// defaultShell is empty, because a window on this system inherits the PATH of
// the user. No shell query is needed, so none runs.
func defaultShell() string { return "" }
