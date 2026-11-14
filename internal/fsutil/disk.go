package fsutil

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
)

// IsSafeFilesystem returns true if the filesystem supports POSIX permissions,
// locking, and symlinks reliably. The list is an allowlist: an unknown
// filesystem is refused, so a migration can never silently lose metadata.
// NTFS holds junctions and file identity, so it is the one Windows entry.
// exFAT and FAT32 do neither, and stay refused on every system.
func IsSafeFilesystem(fstype string) bool {
	norm := strings.ToLower(strings.TrimSpace(fstype))
	switch norm {
	case "apfs", "hfs", "hfs+", "ext4", "ext3", "ext2", "btrfs", "zfs", "xfs", "f2fs", "bcachefs", "ntfs":
		return true
	default:
		return false
	}
}

// FilesystemHint names the filesystems the allowlist accepts on this system.
func FilesystemHint() string { return filesystemHint }

// IsSameDevice checks if two paths reside on the same filesystem/device.
func IsSameDevice(path1, path2 string) (bool, error) {
	if _, err := os.Stat(path1); err != nil {
		return false, err
	}
	if _, err := os.Stat(path2); err != nil {
		return false, err
	}
	return sameDevice(path1, path2)
}

// dirReaders bounds the folders read at once across every call. A store holds
// hundreds of thousands of files, and the walk waits on the disk for each one.
// One pool serves every scan, because two readers on one disk make each other
// slower.
const dirReaders = 8

var dirSlots = make(chan struct{}, dirReaders)

// dirSizer sums one tree with the shared readers.
type dirSizer struct {
	ctx   context.Context
	total atomic.Int64
	wg    sync.WaitGroup
}

// DirSize calculates the total size of files under root.
// A symlinked root is resolved, so a migrated cache reports the size of the
// data it now points to. Symlinks inside the tree are not followed, to avoid
// cycles and double counting.
func DirSize(ctx context.Context, root string) (int64, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}

	resolved, err := filepath.EvalSymlinks(root)
	if err != nil {
		return 0, err
	}

	s := &dirSizer{ctx: ctx}
	s.walk(resolved)
	s.wg.Wait()
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	return s.total.Load(), nil
}

// walk adds the files of one folder and hands every subfolder to a reader. An
// unreadable folder adds nothing, as before: a store can hold sockets and
// folders that the user cannot read.
func (s *dirSizer) walk(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if s.ctx.Err() != nil {
			return
		}
		if entry.Type()&os.ModeSymlink != 0 {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		if entry.IsDir() {
			s.wg.Add(1)
			go func() {
				defer s.wg.Done()
				dirSlots <- struct{}{}
				defer func() { <-dirSlots }()
				s.walk(path)
			}()
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		s.total.Add(info.Size())
	}
}
