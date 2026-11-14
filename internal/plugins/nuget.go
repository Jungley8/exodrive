package plugins

import (
	"os"
	"path/filepath"
	"strings"

	"exodrive/internal/core"
)

// nugetPackages returns the folder that holds the packages that NuGet
// downloaded. NUGET_PACKAGES names it, and ~/.nuget/packages is the default.
func nugetPackages() string {
	if dir := strings.TrimSpace(os.Getenv("NUGET_PACKAGES")); dir != "" {
		return dir
	}
	return homePath(".nuget", "packages")
}

// nugetFolders moves the global package folder. Every project reads it, and
// NuGet downloads a package again when a restore needs it.
var nugetFolders = []linkedRow{
	{
		id:       "nuget.packages",
		title:    "NuGet Packages",
		path:     nugetPackages,
		category: core.CategoryPackageStore,
		subdir:   filepath.Join("nuget", "packages"),
		warning:  "Stop every dotnet build before the move. NuGet downloads the packages again when it needs them.",
	},
}

// NewNuGetProvider moves the NuGet global package folder.
func NewNuGetProvider() *linkedProvider {
	return &linkedProvider{
		tool: "nuget",
		lock: []string{"dotnet", "msbuild", "nuget"},
		rows: nugetFolders,
	}
}
