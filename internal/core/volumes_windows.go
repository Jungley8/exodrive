//go:build windows

package core

import (
	"os"
	"path/filepath"
	"strings"

	"exodrive/internal/fsutil"
)

// volumeSpaces lists the ready drives other than the one that holds the home
// folder. The name is the drive letter, such as D:.
func volumeSpaces() []VolumeSpace {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	homeRoot := fsutil.VolumeRoot(home)

	var spaces []VolumeSpace
	for _, root := range fsutil.DriveRoots() {
		if strings.EqualFold(root, homeRoot) {
			continue
		}
		free, total, _, usageErr := fsutil.GetDiskUsage(root)
		if usageErr != nil || total <= 0 {
			continue
		}
		spaces = append(spaces, VolumeSpace{Name: root, Path: root, Free: free, Total: total})
	}
	return spaces
}

// volumeMount returns the drive root that holds path, or "".
func volumeMount(path string) string {
	vol := filepath.VolumeName(path)
	if vol == "" {
		return ""
	}
	return vol + string(filepath.Separator)
}
