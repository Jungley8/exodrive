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

// ExternalVolumes lists the mounted volumes, without the system volume. The
// window uses the list to tell the user when no external disk is connected.
func ExternalVolumes() []string {
	entries, err := os.ReadDir(VolumesRoot)
	if err != nil {
		return nil
	}

	var names []string
	for _, entry := range entries {
		// The boot volume appears as a link to /. A name with a link holds no
		// data, so the user cannot migrate a cache into it.
		info, statErr := os.Lstat(filepath.Join(VolumesRoot, entry.Name()))
		if statErr != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			continue
		}
		names = append(names, entry.Name())
	}
	sort.Strings(names)
	return names
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

// targetMissingError explains why a target folder is missing. A disconnected
// volume is the common cause, so it gets its own instruction.
func targetMissingError(path string) error {
	if mount := volumeMount(path); mount != "" {
		if _, err := os.Stat(mount); os.IsNotExist(err) {
			return &TargetError{
				Volume:  filepath.Base(mount),
				Path:    path,
				Message: "The external volume " + filepath.Base(mount) + " is not connected. Connect it, then check again.",
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

// VolumeSpace is a mounted external volume and the space it offers.
type VolumeSpace struct {
	Name        string
	Path        string
	Free, Total int64
}

// SpaciousVolumes lists the external volumes with their space, the one with the
// most free space first. A volume whose space cannot be read is left out: a
// number the window cannot check is worse than no number.
func SpaciousVolumes() []VolumeSpace {
	names := ExternalVolumes()
	spaces := make([]VolumeSpace, 0, len(names))
	for _, name := range names {
		path := filepath.Join(VolumesRoot, name)
		free, total, _, err := fsutil.GetDiskUsage(path)
		if err != nil || total <= 0 {
			continue
		}
		spaces = append(spaces, VolumeSpace{Name: name, Path: path, Free: free, Total: total})
	}
	sort.SliceStable(spaces, func(i, j int) bool { return spaces[i].Free > spaces[j].Free })
	return spaces
}
