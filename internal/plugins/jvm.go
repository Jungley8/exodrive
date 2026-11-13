package plugins

import (
	"os"
	"path/filepath"
	"strings"

	"exodrive/internal/core"
)

// mavenHome returns the folder that holds the Maven local repository.
func mavenHome() string { return homePath(".m2") }

// gradleHome returns the Gradle user home. GRADLE_USER_HOME names it, and
// ~/.gradle is the default.
func gradleHome() string {
	if root := strings.TrimSpace(os.Getenv("GRADLE_USER_HOME")); root != "" {
		return root
	}
	return homePath(".gradle")
}

var mavenFolders = []linkedRow{
	{
		id:       "maven.repository",
		title:    "Maven Local Repository",
		path:     func() string { return under(mavenHome(), "repository") },
		category: core.CategoryPackageStore,
		subdir:   filepath.Join("maven", "repository"),
		warning:  "Stop every Maven build before the move.",
	},
}

var gradleFolders = []linkedRow{
	{
		id:       "gradle.caches",
		title:    "Gradle Caches",
		path:     func() string { return under(gradleHome(), "caches") },
		category: core.CategoryPackageStore,
		subdir:   filepath.Join("gradle", "caches"),
		warning:  "Stop every Gradle build before the move. Stop the daemon with gradle --stop.",
	},
	{
		id:       "gradle.wrapper",
		title:    "Gradle Wrapper Distributions",
		path:     func() string { return under(gradleHome(), filepath.Join("wrapper", "dists")) },
		category: core.CategoryPackageStore,
		subdir:   filepath.Join("gradle", "wrapper"),
		warning:  "Stop every Gradle build before the move. Exodrive moves the downloaded distributions.",
	},
}

// NewMavenProvider moves the Maven local repository.
func NewMavenProvider() *linkedProvider {
	return &linkedProvider{
		tool: "maven",
		lock: []string{"mvn", "mvnd"},
		rows: mavenFolders,
	}
}

// NewGradleProvider moves the Gradle caches and the wrapper distributions.
func NewGradleProvider() *linkedProvider {
	return &linkedProvider{
		tool: "gradle",
		lock: []string{"gradle", "GradleDaemon"},
		rows: gradleFolders,
	}
}
