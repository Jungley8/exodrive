//go:build !windows

package core

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"exodrive/internal/fsutil"
)

// VolumesRoot holds the mount points. macOS uses /Volumes. The tests override
// it to run without a real volume.
var VolumesRoot = "/Volumes"

// volumeSpaces lists the mounted folders with their space. A plain file and the
// boot volume link hold no disk to choose, and a volume whose space cannot be
// read is left out.
func volumeSpaces() []VolumeSpace {
	entries, err := os.ReadDir(VolumesRoot)
	if err != nil {
		return nil
	}

	var spaces []VolumeSpace
	for _, entry := range entries {
		path := filepath.Join(VolumesRoot, entry.Name())
		info, statErr := os.Lstat(path)
		if statErr != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			continue
		}
		free, total, _, usageErr := fsutil.GetDiskUsage(path)
		if usageErr != nil || total <= 0 {
			continue
		}
		spaces = append(spaces, VolumeSpace{Name: entry.Name(), Path: path, Free: free, Total: total})
	}
	sort.SliceStable(spaces, func(i, j int) bool { return spaces[i].Name < spaces[j].Name })
	return spaces
}

// volumeMount returns the mount point that holds path, or "".
func volumeMount(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		return ""
	}
	root := strings.TrimRight(VolumesRoot, string(filepath.Separator))
	if !strings.HasPrefix(abs, root+string(filepath.Separator)) {
		return ""
	}
	rest := strings.TrimPrefix(abs, root+string(filepath.Separator))
	name, _, _ := strings.Cut(rest, string(filepath.Separator))
	if name == "" {
		return ""
	}
	return filepath.Join(root, name)
}
