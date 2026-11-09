package main

import (
	"exodrive/desktop"
	"github.com/oneo-me/nsgo"
)

func main() {
	nsgo.Run(desktop.Root, nsgo.WindowSize(860, 680))
}
