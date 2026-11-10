package fsutil

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// CopyProgressCallback reports the cumulative copied bytes.
type CopyProgressCallback func(copiedBytes, totalBytes int64)

// CopyDir recursively copies src directory to dst directory.
// It preserves each entry's permissions and modification time. A failed copy
// leaves the partial destination in place for inspection; callers must not
// replace the source until CopyDir returns nil.
func CopyDir(src, dst string, totalBytes int64, progress CopyProgressCallback) error {
	srcInfo, err := os.Stat(src)
	if err != nil {
		return err
	}
	if !srcInfo.IsDir() {
		return fmt.Errorf("source %s is not a directory", src)
	}

	absSrc, err := filepath.Abs(src)
	if err != nil {
		return err
	}
	absDst, err := filepath.Abs(dst)
	if err != nil {
		return err
	}
	if absDst == absSrc || strings.HasPrefix(absDst, absSrc+string(filepath.Separator)) {
		return fmt.Errorf("destination %s is inside source %s", dst, src)
	}

	// A destination that holds a link or data would merge two trees. The copy
	// would then overwrite files it does not own, and VerifyTree could pass on
	// a mix of both. Refuse it.
	switch info, statErr := os.Lstat(dst); {
	case statErr == nil && info.Mode()&os.ModeSymlink != 0:
		return fmt.Errorf("destination %s is a symbolic link: choose a folder", dst)
	case statErr == nil && !info.IsDir():
		return fmt.Errorf("destination %s is a file: choose a folder", dst)
	case statErr == nil:
		entries, readErr := os.ReadDir(dst)
		if readErr != nil {
			return fmt.Errorf("inspect destination %s: %w", dst, readErr)
		}
		if len(entries) > 0 {
			return fmt.Errorf("destination %s is not empty: remove its contents, then run the migration again", dst)
		}
	case !os.IsNotExist(statErr):
		return fmt.Errorf("inspect destination %s: %w", dst, statErr)
	}

	if err := os.MkdirAll(dst, 0755); err != nil {
		return err
	}

	var copiedBytes int64
	type dirMeta struct {
		src, dst string
		mode     os.FileMode
	}
	var dirs []dirMeta

	err = filepath.Walk(src, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}

		targetPath := filepath.Join(dst, relPath)

		// Handle symlink: copy the link itself, never its target.
		if info.Mode()&os.ModeSymlink != 0 {
			linkTarget, err := os.Readlink(path)
			if err != nil {
				return err
			}
			return os.Symlink(linkTarget, targetPath)
		}

		// Handle directory.
		if info.IsDir() {
			// Ensure writable during copy even if source is read-only.
			if err := os.MkdirAll(targetPath, 0755); err != nil {
				return err
			}
			dirs = append(dirs, dirMeta{src: path, dst: targetPath, mode: info.Mode()})
			return nil
		}

		// Handle regular file.
		if err := copyFile(path, targetPath, info); err != nil {
			return fmt.Errorf("copy file %s -> %s: %w", path, targetPath, err)
		}

		copiedBytes += info.Size()
		if progress != nil {
			progress(copiedBytes, totalBytes)
		}

		return nil
	})

	if err != nil {
		return err
	}

	// Restore directory modes and times last, deepest first, because writing
	// into a directory updates its modification time.
	for i := len(dirs) - 1; i >= 0; i-- {
		info, statErr := os.Stat(dirs[i].src)
		if statErr != nil {
			return statErr
		}
		if err := os.Chmod(dirs[i].dst, dirs[i].mode); err != nil {
			return fmt.Errorf("restore directory mode %s: %w", dirs[i].dst, err)
		}
		if err := os.Chtimes(dirs[i].dst, info.ModTime(), info.ModTime()); err != nil {
			return fmt.Errorf("restore directory time %s: %w", dirs[i].dst, err)
		}
	}

	return nil
}

// CopyFile copies a single file from src to dst, preserving permissions and
// modification time and reporting progress. On failure it removes the partial
// destination file.
func CopyFile(src, dst string, totalBytes int64, progress CopyProgressCallback) error {
	absSrc, err := filepath.Abs(src)
	if err != nil {
		return err
	}
	absDst, err := filepath.Abs(dst)
	if err != nil {
		return err
	}
	if absSrc == absDst {
		return fmt.Errorf("source and destination are the same file: %s", src)
	}

	srcInfo, err := os.Stat(src)
	if err != nil {
		return err
	}
	if srcInfo.IsDir() {
		return fmt.Errorf("source %s is a directory, use CopyDir", src)
	}

	if totalBytes <= 0 {
		totalBytes = srcInfo.Size()
	}

	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}

	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, srcInfo.Mode()|0200)
	if err != nil {
		return err
	}

	fail := func(cause error) error {
		_ = out.Close()
		_ = os.Remove(dst)
		return cause
	}

	buf := make([]byte, 1024*1024)
	var copied int64
	for {
		n, readErr := in.Read(buf)
		if n > 0 {
			if _, writeErr := out.Write(buf[:n]); writeErr != nil {
				return fail(writeErr)
			}
			copied += int64(n)
			if progress != nil {
				progress(copied, totalBytes)
			}
		}
		if readErr != nil {
			if readErr == io.EOF {
				break
			}
			return fail(readErr)
		}
	}

	// Flush the data before the caller replaces the source with a link.
	if err := out.Sync(); err != nil {
		return fail(err)
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(dst)
		return err
	}
	if err := os.Chmod(dst, srcInfo.Mode()); err != nil {
		_ = os.Remove(dst)
		return err
	}
	if err := os.Chtimes(dst, srcInfo.ModTime(), srcInfo.ModTime()); err != nil {
		_ = os.Remove(dst)
		return err
	}
	return nil
}

func copyFile(src, dst string, info os.FileInfo) (err error) {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode()|0200)
	if err != nil {
		return err
	}

	fail := func(cause error) error {
		_ = out.Close()
		_ = os.Remove(dst)
		return cause
	}

	if _, err = io.Copy(out, in); err != nil {
		return fail(err)
	}
	if err = out.Sync(); err != nil {
		return fail(err)
	}
	if err = out.Close(); err != nil {
		_ = os.Remove(dst)
		return err
	}

	// Restore the original file mode and time.
	if err := os.Chmod(dst, info.Mode()); err != nil {
		_ = os.Remove(dst)
		return err
	}
	if err := os.Chtimes(dst, info.ModTime(), info.ModTime()); err != nil {
		_ = os.Remove(dst)
		return err
	}
	return nil
}

// VerifyTree checks that dst matches src: the same regular files, the same
// total size, and the same symbolic links with the same targets. A provider
// calls it before it replaces the source with a link.
func VerifyTree(src, dst string) error {
	srcTree, err := scanTree(src)
	if err != nil {
		return err
	}
	dstTree, err := scanTree(dst)
	if err != nil {
		return err
	}
	if srcTree.files != dstTree.files {
		return fmt.Errorf("verification failed: %s has %d files, %s has %d", dst, dstTree.files, src, srcTree.files)
	}
	if srcTree.bytes != dstTree.bytes {
		return fmt.Errorf("verification failed: %s has %d bytes, %s has %d", dst, dstTree.bytes, src, srcTree.bytes)
	}
	if len(srcTree.links) != len(dstTree.links) {
		return fmt.Errorf("verification failed: %s has %d symbolic links, %s has %d", dst, len(dstTree.links), src, len(srcTree.links))
	}
	for rel, target := range srcTree.links {
		copied, ok := dstTree.links[rel]
		if !ok {
			return fmt.Errorf("verification failed: %s is missing the symbolic link %s", dst, rel)
		}
		if copied != target {
			return fmt.Errorf("verification failed: %s points to %s, the source points to %s", filepath.Join(dst, rel), copied, target)
		}
	}
	return nil
}

// treeSummary holds what VerifyTree compares.
type treeSummary struct {
	files int64
	bytes int64
	// links maps the path relative to the root to the link target.
	links map[string]string
}

func scanTree(root string) (treeSummary, error) {
	summary := treeSummary{links: make(map[string]string)}
	err := filepath.Walk(root, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == root {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			target, err := os.Readlink(path)
			if err != nil {
				return err
			}
			summary.links[rel] = target
			return nil
		}
		if info.IsDir() {
			return nil
		}
		summary.files++
		summary.bytes += info.Size()
		return nil
	})
	return summary, err
}
