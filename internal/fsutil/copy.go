package fsutil

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// CopyProgressCallback reports the cumulative copied bytes.
type CopyProgressCallback func(copiedBytes, totalBytes int64)

// CopyDir recursively copies src directory to dst directory.
// It preserves file attributes while ensuring created parent directories are writable during copy.
func CopyDir(src, dst string, totalBytes int64, progress CopyProgressCallback) error {
	srcInfo, err := os.Stat(src)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(dst, 0755); err != nil {
		return err
	}

	var copiedBytes int64

	err = filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}

		targetPath := filepath.Join(dst, relPath)

		// Handle symlink
		if info.Mode()&os.ModeSymlink != 0 {
			linkTarget, err := os.Readlink(path)
			if err != nil {
				return err
			}
			return os.Symlink(linkTarget, targetPath)
		}

		// Handle directory
		if info.IsDir() {
			// Ensure writable during copy even if source is read-only
			return os.MkdirAll(targetPath, 0755)
		}

		// Handle regular file
		if err := copyFile(path, targetPath, info.Mode()); err != nil {
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

	_ = srcInfo
	return nil
}

// CopyFile copies a single file from src to dst, preserving permissions and reporting progress.
func CopyFile(src, dst string, totalBytes int64, progress CopyProgressCallback) (err error) {
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
	defer func() {
		if cerr := in.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, srcInfo.Mode()|0200)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := out.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}()

	buf := make([]byte, 1024*1024)
	var copied int64
	for {
		n, rerr := in.Read(buf)
		if n > 0 {
			if _, werr := out.Write(buf[:n]); werr != nil {
				return werr
			}
			copied += int64(n)
			if progress != nil {
				progress(copied, totalBytes)
			}
		}
		if rerr != nil {
			if rerr == io.EOF {
				break
			}
			return rerr
		}
	}

	return os.Chmod(dst, srcInfo.Mode())
}

func copyFile(src, dst string, mode os.FileMode) (err error) {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := in.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode|0200)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := out.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}()

	if _, err = io.Copy(out, in); err != nil {
		return err
	}

	// Restore original file mode permissions
	return os.Chmod(dst, mode)
}
