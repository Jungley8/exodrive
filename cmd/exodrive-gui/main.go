package main

import (
	"context"

	"exodrive/desktop"
	"exodrive/internal/plugins"
	"github.com/oneo-me/nsgo"
)

func main() {
	// A window started from Finder inherits the launchd PATH, which holds no
	// Homebrew or nvm folder. Read the login shell PATH before the first scan.
	plugins.UseLoginShellPath(context.Background())

	nsgo.Run(desktop.Root, nsgo.WindowSize(860, 680))
}
