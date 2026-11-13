package plugins

import (
	"path/filepath"

	"exodrive/internal/core"
)

// xcodeSimulatorReason explains why Xcode's simulator folder must stay on this
// Mac. A link to an external volume stops CoreSimulatorService from writing its
// device store.
const xcodeSimulatorReason = "Exodrive cannot move this folder. " +
	"CoreSimulatorService cannot write to an external volume. " +
	"A link stops Xcode from creating simulator devices. " +
	"It also breaks the Files app in the simulator. " +
	"Keep this folder on this Mac."

// xcodeFolders lists the folders that Xcode writes under the home folder.
//
// DerivedData, Archives, DeviceSupport, and Previews are safe to move: Xcode
// reads them through the path. The simulator folder is not.
var xcodeFolders = []linkedRow{
	{
		id:       "xcode.derived-data",
		title:    "Xcode Derived Data",
		path:     func() string { return homePath("Library", "Developer", "Xcode", "DerivedData") },
		category: core.CategoryBuildCache,
		subdir:   filepath.Join("xcode", "DerivedData"),
		warning:  "Quit Xcode before the move.",
	},
	{
		id:       "xcode.archives",
		title:    "Xcode Archives",
		path:     func() string { return homePath("Library", "Developer", "Xcode", "Archives") },
		category: core.CategoryBuildCache,
		subdir:   filepath.Join("xcode", "Archives"),
		warning:  "Quit Xcode before the move. Xcode Organizer reads this folder.",
	},
	{
		id:       "xcode.device-support-ios",
		title:    "Xcode iOS Device Support",
		path:     func() string { return homePath("Library", "Developer", "Xcode", "iOS DeviceSupport") },
		category: core.CategoryBuildCache,
		subdir:   filepath.Join("xcode", "DeviceSupport", "iOS"),
		warning:  "Quit Xcode before the move. A debug session reads this folder.",
	},
	{
		id:       "xcode.device-support-watchos",
		title:    "Xcode watchOS Device Support",
		path:     func() string { return homePath("Library", "Developer", "Xcode", "watchOS DeviceSupport") },
		category: core.CategoryBuildCache,
		subdir:   filepath.Join("xcode", "DeviceSupport", "watchOS"),
		warning:  "Quit Xcode before the move. A debug session reads this folder.",
	},
	{
		id:       "xcode.device-support-tvos",
		title:    "Xcode tvOS Device Support",
		path:     func() string { return homePath("Library", "Developer", "Xcode", "tvOS DeviceSupport") },
		category: core.CategoryBuildCache,
		subdir:   filepath.Join("xcode", "DeviceSupport", "tvOS"),
		warning:  "Quit Xcode before the move. A debug session reads this folder.",
	},
	{
		id:       "xcode.device-support-visionos",
		title:    "Xcode visionOS Device Support",
		path:     func() string { return homePath("Library", "Developer", "Xcode", "visionOS DeviceSupport") },
		category: core.CategoryBuildCache,
		subdir:   filepath.Join("xcode", "DeviceSupport", "visionOS"),
		warning:  "Quit Xcode before the move. A debug session reads this folder.",
	},
	{
		id:       "xcode.previews",
		title:    "Xcode Previews Data",
		path:     func() string { return homePath("Library", "Developer", "Xcode", "UserData", "Previews") },
		category: core.CategoryBuildCache,
		subdir:   filepath.Join("xcode", "Previews"),
		warning:  "Quit Xcode before the move. Xcode Previews reads this folder.",
	},
	{
		id:       "xcode.app-cache",
		title:    "Xcode App Cache",
		path:     func() string { return homePath("Library", "Caches", "com.apple.dt.Xcode") },
		category: core.CategoryBuildCache,
		subdir:   filepath.Join("xcode", "AppCache"),
		warning:  "Quit Xcode before the move. Xcode writes new files here again.",
	},
	{
		id:       "xcode.simulator-devices",
		title:    "Simulator Devices",
		path:     func() string { return homePath("Library", "Developer", "CoreSimulator", "Devices") },
		category: core.CategoryVirtualDisk,
		blocked:  xcodeSimulatorReason,
	},
}

// NewXcodeProvider moves Xcode build output, archives, device support, and
// preview data. It reports the simulator folder without a move.
func NewXcodeProvider() *linkedProvider {
	return &linkedProvider{
		tool: "xcode",
		lock: []string{"Xcode", "xcodebuild", "XCBuildService"},
		rows: xcodeFolders,
	}
}
