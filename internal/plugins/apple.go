package plugins

import (
	"os"
	"path/filepath"
	"strings"

	"exodrive/internal/core"
)

// cocoapodsHome returns the folder that holds the CocoaPods spec repos.
// CP_HOME_DIR names it, and ~/.cocoapods is the default.
func cocoapodsHome() string {
	if root := strings.TrimSpace(os.Getenv("CP_HOME_DIR")); root != "" {
		return root
	}
	return homePath(".cocoapods")
}

var cocoapodsFolders = []linkedRow{
	{
		id:       "cocoapods.cache",
		title:    "CocoaPods Cache",
		path:     func() string { return homePath("Library", "Caches", "CocoaPods") },
		category: core.CategoryPackageStore,
		subdir:   filepath.Join("cocoapods", "cache"),
		warning:  "Quit Xcode before the move. CocoaPods downloads the pods again when it needs them.",
	},
	{
		id:       "cocoapods.repos",
		title:    "CocoaPods Spec Repos",
		path:     func() string { return under(cocoapodsHome(), "repos") },
		category: core.CategoryPackageStore,
		subdir:   filepath.Join("cocoapods", "repos"),
		warning:  "Quit Xcode before the move. CocoaPods downloads the specs again when it needs them.",
	},
}

var swiftpmFolders = []linkedRow{
	{
		id:       "swiftpm.cache",
		title:    "Swift Package Cache",
		path:     func() string { return homePath("Library", "Caches", "org.swift.swiftpm") },
		category: core.CategoryPackageStore,
		subdir:   filepath.Join("swiftpm", "cache"),
		warning:  "Quit Xcode before the move. SwiftPM downloads the packages again when it needs them.",
	},
}

// NewCocoaPodsProvider moves the CocoaPods cache and the spec repos.
func NewCocoaPodsProvider() *linkedProvider {
	return &linkedProvider{
		tool: "cocoapods",
		lock: []string{"pod"},
		rows: cocoapodsFolders,
	}
}

// NewSwiftPMProvider moves the Swift Package cache.
func NewSwiftPMProvider() *linkedProvider {
	return &linkedProvider{
		tool: "swiftpm",
		lock: []string{"swift", "swiftc", "xcodebuild"},
		rows: swiftpmFolders,
	}
}
