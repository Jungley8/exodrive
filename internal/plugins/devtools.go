package plugins

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"exodrive/internal/core"
)

// devtoolsFolders moves the caches that download and build tools keep:
// Electron binaries, node-gyp headers, and the TypeScript build cache.
var devtoolsFolders = []linkedRow{
	{
		id:       "devtools.electron",
		title:    "Electron Downloads",
		path:     func() string { return osCacheDir("electron") },
		category: core.CategoryPackageStore,
		subdir:   filepath.Join("devtools", "electron"),
		warning:  "Stop every build that downloads Electron before the move. The build downloads what it needs again.",
	},
	{
		id:       "devtools.node-gyp",
		title:    "node-gyp Headers",
		path:     func() string { return osCacheDir("node-gyp") },
		category: core.CategoryBuildCache,
		subdir:   filepath.Join("devtools", "node-gyp"),
		warning:  "Stop every build that compiles a native module before the move. node-gyp downloads the headers again when it needs them.",
	},
	{
		id:       "devtools.typescript",
		title:    "TypeScript Cache",
		path:     func() string { return osCacheDir("typescript") },
		category: core.CategoryBuildCache,
		subdir:   filepath.Join("devtools", "typescript"),
		warning:  "Stop every tsc build before the move. tsc writes the cache again when it needs it.",
	},
}

// NewDevtoolsProvider moves the caches that download tools keep. No process
// lock names node here: node runs long-lived programs, so the lock would stop
// a move on a machine that builds nothing.
func NewDevtoolsProvider() *linkedProvider {
	return &linkedProvider{
		tool: "devtools",
		rows: devtoolsFolders,
	}
}

// osCacheDir names the per-user cache folder of one tool. The system decides
// where the folder lives, so Exodrive never guesses a path that the tool does
// not read.
func osCacheDir(name string) string {
	switch runtime.GOOS {
	case "windows":
		appData := strings.TrimSpace(os.Getenv("LOCALAPPDATA"))
		if appData == "" {
			return ""
		}
		return filepath.Join(appData, name)
	case "darwin":
		return homePath("Library", "Caches", name)
	default:
		if xdg := strings.TrimSpace(os.Getenv("XDG_CACHE_HOME")); xdg != "" {
			return filepath.Join(xdg, name)
		}
		return homePath(".cache", name)
	}
}
