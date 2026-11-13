//go:build unix

package plugins

// defaultShell is the shell that every macOS and Linux system provides.
func defaultShell() string { return "/bin/zsh" }
