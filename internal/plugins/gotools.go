package plugins

import (
	"os"
	"path/filepath"
	"strings"

	"exodrive/internal/core"
)

// golangciLintCache returns the folder where golangci-lint keeps its analysis
// cache. GOLANGCI_LINT_CACHE names it, and the system cache folder is the
// default.
func golangciLintCache() string {
	if dir := strings.TrimSpace(os.Getenv("GOLANGCI_LINT_CACHE")); dir != "" {
		return dir
	}
	return osCacheDir("golangci-lint")
}

// goplsCache returns the folder where gopls keeps its analysis cache. gopls
// reads the system cache folder and takes no variable that names another one.
func goplsCache() string {
	return osCacheDir("gopls")
}

// goToolFolders move the caches that the Go analysis tools fill. Each tool
// writes the cache again, so a wrong move costs analysis time and no data.
var goToolFolders = []linkedRow{
	{
		id:       "golangci-lint.cache",
		title:    "golangci-lint Cache",
		path:     golangciLintCache,
		category: core.CategoryBuildCache,
		subdir:   filepath.Join("go-tools", "golangci-lint"),
		warning:  "Stop every golangci-lint run before the move. The next run writes the cache again.",
	},
	{
		id:       "gopls.cache",
		title:    "gopls Cache",
		path:     goplsCache,
		category: core.CategoryBuildCache,
		subdir:   filepath.Join("go-tools", "gopls"),
		warning:  "Stop every editor session that runs gopls before the move. The next analysis writes the cache again.",
	},
}

// NewGolangciLintProvider moves the golangci-lint analysis cache. The lock
// names the tool: a lint run writes the cache, and a run is short.
func NewGolangciLintProvider() *linkedProvider {
	return &linkedProvider{tool: "golangci-lint", lock: []string{"golangci-lint"}, rows: []linkedRow{goToolFolders[0]}}
}

// NewGoplsProvider moves the gopls analysis cache. No lock names gopls: an
// editor keeps it running, so the lock would stop a move on every machine with
// an open editor.
func NewGoplsProvider() *linkedProvider {
	return &linkedProvider{tool: "gopls", rows: []linkedRow{goToolFolders[1]}}
}
