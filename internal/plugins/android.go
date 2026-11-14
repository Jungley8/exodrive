package plugins

import (
	"os"
	"path/filepath"
	"strings"

	"exodrive/internal/core"
)

// androidFolders lists the Android stores that grow without limit. The build
// cache and the emulator images each keep their own process lock: a Gradle
// build writes one, and the emulator writes the other.
var androidFolders = []linkedRow{
	{
		id:       "android.build-cache",
		title:    "Android Build Cache",
		path:     func() string { return homePath(".android", "build-cache") },
		category: core.CategoryBuildCache,
		subdir:   filepath.Join("android", "build-cache"),
		warning:  "Stop every Gradle build before the move. Stop the daemon with gradle --stop.",
	},
	{
		id:       "android.avd",
		title:    "Android Emulator Images",
		path:     androidAVDs,
		category: core.CategoryVirtualDisk,
		subdir:   filepath.Join("android", "avd"),
		warning:  "Stop the emulator before the move. A slow disk slows every launch, so use a fast external SSD.",
		lock:     []string{"emulator", "qemu-system-aarch64", "qemu-system-x86_64"},
	},
}

// androidAVDs returns the folder that holds the emulator images. ANDROID_AVD_HOME
// names it, and ~/.android/avd is the default.
func androidAVDs() string {
	if dir := strings.TrimSpace(os.Getenv("ANDROID_AVD_HOME")); dir != "" {
		return dir
	}
	return homePath(".android", "avd")
}

// NewAndroidProvider moves the Android build cache.
func NewAndroidProvider() *linkedProvider {
	return &linkedProvider{
		tool: "android",
		lock: []string{"gradle", "GradleDaemon"},
		rows: androidFolders,
	}
}
