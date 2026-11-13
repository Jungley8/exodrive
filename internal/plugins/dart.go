package plugins

import (
	"os"
	"path/filepath"
	"strings"

	"exodrive/internal/core"
)

// pubHome returns the folder that holds the Dart and Flutter package cache.
// PUB_CACHE names it, and ~/.pub-cache is the default.
func pubHome() string {
	if root := strings.TrimSpace(os.Getenv("PUB_CACHE")); root != "" {
		return root
	}
	return homePath(".pub-cache")
}

// dartFolders moves the downloaded packages and git checkouts. The `bin/` and
// `global_packages/` folders stay: a global command on PATH must keep working
// when the drive is away.
var dartFolders = []linkedRow{
	{
		id:       "dart.pub-cache",
		title:    "Dart Pub Package Cache",
		path:     func() string { return under(pubHome(), "hosted") },
		category: core.CategoryPackageStore,
		subdir:   filepath.Join("dart", "hosted"),
		warning:  "Stop every Dart and Flutter build before the move.",
	},
	{
		id:       "dart.pub-git",
		title:    "Dart Pub Git Cache",
		path:     func() string { return under(pubHome(), "git") },
		category: core.CategoryPackageStore,
		subdir:   filepath.Join("dart", "git"),
		warning:  "Stop every Dart and Flutter build before the move.",
	},
}

// NewDartProvider moves the pub package cache.
func NewDartProvider() *linkedProvider {
	return &linkedProvider{
		tool: "dart",
		lock: []string{"dart", "flutter", "pub"},
		rows: dartFolders,
	}
}
