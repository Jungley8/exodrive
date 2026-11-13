package plugins

import (
	"path/filepath"

	"exodrive/internal/core"
)

// androidFolders lists the Android build outputs that grow without limit. The
// emulator data stays: an emulator on another volume runs far slower.
var androidFolders = []linkedRow{
	{
		id:       "android.build-cache",
		title:    "Android Build Cache",
		path:     func() string { return homePath(".android", "build-cache") },
		category: core.CategoryBuildCache,
		subdir:   filepath.Join("android", "build-cache"),
		warning:  "Stop every Gradle build before the move. Stop the daemon with gradle --stop.",
	},
}

// NewAndroidProvider moves the Android build cache.
func NewAndroidProvider() *linkedProvider {
	return &linkedProvider{
		tool: "android",
		lock: []string{"gradle", "GradleDaemon"},
		rows: androidFolders,
	}
}
