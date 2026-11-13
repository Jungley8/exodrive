package plugins

import (
	"os"
	"path/filepath"
	"strings"

	"exodrive/internal/core"
)

// homebrewCache returns the folder that holds the Homebrew cache.
// HOMEBREW_CACHE names it, and ~/Library/Caches/Homebrew is the default.
func homebrewCache() string {
	if root := strings.TrimSpace(os.Getenv("HOMEBREW_CACHE")); root != "" {
		return root
	}
	return homePath("Library", "Caches", "Homebrew")
}

// homebrewFolders moves the downloaded bottles and source archives. The `api/`
// folder stays: it holds the formula index that every brew command reads.
var homebrewFolders = []linkedRow{
	{
		id:       "homebrew.downloads",
		title:    "Homebrew Downloads",
		path:     func() string { return under(homebrewCache(), "downloads") },
		category: core.CategoryPackageStore,
		subdir:   filepath.Join("homebrew", "downloads"),
		warning:  "Stop every brew command before the move. Homebrew downloads the files again when it needs them.",
	},
}

// NewHomebrewProvider moves the Homebrew download cache.
func NewHomebrewProvider() *linkedProvider {
	return &linkedProvider{
		tool: "homebrew",
		lock: []string{"brew"},
		rows: homebrewFolders,
	}
}
