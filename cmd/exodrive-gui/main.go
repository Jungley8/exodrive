package main

import (
	"context"

	"exodrive/desktop"
	"exodrive/internal/plugins"
	"github.com/oneo-me/nsgo"
)

// version is the build's version. The release build sets it with
// -ldflags "-X main.version=0.6.0".
var version = "dev"

func main() {
	// A window started from Finder inherits the launchd PATH, which holds no
	// Homebrew or nvm folder. Read the login shell PATH before the first scan.
	plugins.UseLoginShellPath(context.Background())

	desktop.Version = version
	nsgo.Run(desktop.Root, nsgo.WindowSize(900, 720))
}
