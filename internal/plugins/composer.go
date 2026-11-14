package plugins

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"exodrive/internal/core"
)

// composerCache returns the folder that holds the packages that Composer
// downloaded. COMPOSER_CACHE_DIR names it, and the system cache folder is the
// default.
func composerCache() string {
	if dir := strings.TrimSpace(os.Getenv("COMPOSER_CACHE_DIR")); dir != "" {
		return dir
	}
	if runtime.GOOS == "windows" {
		return osCacheDir("Composer")
	}
	return osCacheDir("composer")
}

// composerFolders moves the Composer download cache. Composer writes the
// packages into it again when a project needs them.
var composerFolders = []linkedRow{
	{
		id:       "composer.cache",
		title:    "Composer Cache",
		path:     composerCache,
		category: core.CategoryPackageStore,
		subdir:   filepath.Join("composer", "cache"),
		warning:  "Stop every composer command before the move. Composer downloads the packages again when it needs them.",
	},
}

// NewComposerProvider moves the Composer download cache.
func NewComposerProvider() *linkedProvider {
	return &linkedProvider{
		tool: "composer",
		lock: []string{"composer"},
		rows: composerFolders,
	}
}
