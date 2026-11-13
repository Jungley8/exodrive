package core

import (
	"os"
	"path/filepath"
	"sort"
)

// volumeSpaces, volumeMount, and ExternalVolumes are implemented per system:
// macOS and Linux list mount points, Windows lists drive roots.

// VolumeSpace is a mounted external volume and the space it offers.
type VolumeSpace struct {
	Name        string
	Path        string
	Free, Total int64
}

// ExternalVolumes lists the mounted volumes, without the system volume. The
// window uses the list to tell the user when no external disk is connected.
func ExternalVolumes() []string {
	spaces := volumeSpaces()
	names := make([]string, 0, len(spaces))
	for _, space := range spaces {
		names = append(names, space.Name)
	}
	sort.Strings(names)
	return names
}

// SpaciousVolumes lists the external volumes with their space, the one with the
// most free space first. A volume whose space cannot be read is left out: a
// number the window cannot check is worse than no number.
func SpaciousVolumes() []VolumeSpace {
	spaces := volumeSpaces()
	sort.SliceStable(spaces, func(i, j int) bool { return spaces[i].Free > spaces[j].Free })
	return spaces
}

// targetMissingError explains why a target folder is missing. A disconnected
// volume is the common cause, so it gets its own instruction.
func targetMissingError(path string) error {
	if mount := volumeMount(path); mount != "" {
		if _, err := os.Stat(mount); os.IsNotExist(err) {
			name := filepath.Base(mount)
			return &TargetError{
				Volume:  name,
				Path:    path,
				Message: "The external volume " + name + " is not connected. Connect it, then check again.",
			}
		}
	}
	return &TargetError{
		Path:    path,
		Message: "The folder " + path + " does not exist. Choose a folder that exists.",
	}
}

// TargetError says why a target folder fails. The window shows Message as it
// is. Volume names the disconnected volume, or is empty; Path is the folder
// that failed the check.
type TargetError struct {
	Volume  string
	Path    string
	Message string
}

func (e *TargetError) Error() string { return e.Message }
