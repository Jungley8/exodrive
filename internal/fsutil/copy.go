package fsutil

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// CopyProgressCallback reports the cumulative copied bytes.
type CopyProgressCallback func(copiedBytes, totalBytes int64)

// MigrateProgress holds the callbacks for the two steps that report progress.
// Either field may be nil.
type MigrateProgress struct {
	// Copy reports the bytes copied.
	Copy CopyProgressCallback
	// Verify reports the bytes verified.
	Verify CopyProgressCallback
}

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

		// Handle regular file. A socket, a FIFO, or a device file cannot be
		// copied or read, so the migration must not start on a tree that holds
		// one.
		if !info.Mode().IsRegular() {
			return fmt.Errorf("special file %s: stop the tool that owns it, then run the migration again", path)
		}
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

// VerifyTree checks that dst matches src: the same regular files with the same
// bytes, the same total size, and the same symbolic links with the same
// targets. A provider calls it before it replaces the source with a link. It
// reads every file in both trees, so a damaged byte stops the migration.
func VerifyTree(src, dst string) error {
	_, err := VerifyTreeSummary(src, dst, nil)
	return err
}

// CopySummary describes a copy that passed the check, with a digest that the
// ledger keeps. `exodrive verify` reads the copy again and compares the digest,
// which works after the local data is gone.
type CopySummary struct {
	Digest TreeDigest
	Files  int64
	Bytes  int64
}

// TreeDigest identifies the content of a copy: every regular file with its size
// and bytes, and every symbolic link with its target. Two copies of the same
// tree produce the same digest on any system.
type TreeDigest string

// VerifyTreeSummary runs the VerifyTree check and returns the digest of the
// copy. The digest costs no extra read: it uses the bytes the check already
// reads.
func VerifyTreeSummary(src, dst string, progress CopyProgressCallback) (CopySummary, error) {
	srcTree, err := scanTree(src)
	if err != nil {
		return CopySummary{}, err
	}
	dstTree, err := scanTree(dst)
	if err != nil {
		return CopySummary{}, err
	}
	if srcTree.files != dstTree.files {
		return CopySummary{}, fmt.Errorf("verification failed: %s has %d files, %s has %d", dst, dstTree.files, src, srcTree.files)
	}
	if srcTree.bytes != dstTree.bytes {
		return CopySummary{}, fmt.Errorf("verification failed: %s has %d bytes, %s has %d", dst, dstTree.bytes, src, srcTree.bytes)
	}

	outer := sha256.New()
	if err := compareContent(src, dst, srcTree, dstTree, progress, outer); err != nil {
		return CopySummary{}, err
	}
	if err := compareLinks(src, dst, srcTree, dstTree, outer); err != nil {
		return CopySummary{}, err
	}

	return CopySummary{
		Digest: TreeDigest(hex.EncodeToString(outer.Sum(nil))),
		Files:  srcTree.files,
		Bytes:  srcTree.bytes,
	}, nil
}

// treeSummary holds what VerifyTree compares.
type treeSummary struct {
	files int64
	bytes int64
	// sizes maps the path relative to the root to the file size.
	sizes map[string]int64
	// links maps the path relative to the root to the link target.
	links map[string]string
}

func scanTree(root string) (treeSummary, error) {
	summary := treeSummary{
		sizes: make(map[string]int64),
		links: make(map[string]string),
	}
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
		if !info.Mode().IsRegular() {
			return fmt.Errorf("special file %s: stop the tool that owns it, then run the migration again", path)
		}
		summary.files++
		summary.bytes += info.Size()
		summary.sizes[rel] = info.Size()
		return nil
	})
	return summary, err
}

// compareContent reads every regular file in both trees and compares the
// bytes. File count and total size alone pass a copy that swapped one file for
// another of the same length. Each file enters outer, so the caller gets a
// digest of the whole tree.
func compareContent(src, dst string, srcTree, dstTree treeSummary, progress CopyProgressCallback, outer io.Writer) error {
	var verified int64
	for _, rel := range sortedKeys(srcTree.sizes) {
		size := srcTree.sizes[rel]
		copiedSize, ok := dstTree.sizes[rel]
		if !ok {
			return fmt.Errorf("verification failed: %s is missing the file %s", dst, rel)
		}
		if copiedSize != size {
			return fmt.Errorf("verification failed: %s has %d bytes, the source file has %d", filepath.Join(dst, rel), copiedSize, size)
		}
		srcSum, err := fileDigest(filepath.Join(src, rel))
		if err != nil {
			return fmt.Errorf("verification failed: %w", err)
		}
		dstSum, err := fileDigest(filepath.Join(dst, rel))
		if err != nil {
			return fmt.Errorf("verification failed: %w", err)
		}
		if !bytes.Equal(srcSum, dstSum) {
			return fmt.Errorf("verification failed: %s differs from the source file", filepath.Join(dst, rel))
		}
		writeFileEntry(outer, rel, size, dstSum)
		verified += size
		if progress != nil {
			progress(verified, srcTree.bytes)
		}
	}
	return nil
}

// compareLinks checks that both trees hold the same symbolic links with the
// same targets, and adds them to the digest.
func compareLinks(src, dst string, srcTree, dstTree treeSummary, outer io.Writer) error {
	if len(srcTree.links) != len(dstTree.links) {
		return fmt.Errorf("verification failed: %s has %d symbolic links, %s has %d", dst, len(dstTree.links), src, len(srcTree.links))
	}
	for _, rel := range sortedKeys(srcTree.links) {
		target := srcTree.links[rel]
		copied, ok := dstTree.links[rel]
		if !ok {
			return fmt.Errorf("verification failed: %s is missing the symbolic link %s", dst, rel)
		}
		if copied != target {
			return fmt.Errorf("verification failed: %s points to %s, the source points to %s", filepath.Join(dst, rel), copied, target)
		}
		writeLinkEntry(outer, rel, copied)
	}
	return nil
}

// writeFileEntry adds one regular file to a tree digest. The path, the size,
// and a separator keep two different trees from writing the same bytes.
func writeFileEntry(w io.Writer, rel string, size int64, sum []byte) {
	_, _ = io.WriteString(w, "file\x00")
	_, _ = io.WriteString(w, rel)
	_, _ = w.Write([]byte{0})
	_, _ = io.WriteString(w, strconv.FormatInt(size, 10))
	_, _ = w.Write([]byte{0})
	_, _ = w.Write(sum)
	_, _ = w.Write([]byte{'\n'})
}

// writeLinkEntry adds one symbolic link and its target to a tree digest.
func writeLinkEntry(w io.Writer, rel, target string) {
	_, _ = io.WriteString(w, "link\x00")
	_, _ = io.WriteString(w, rel)
	_, _ = w.Write([]byte{0})
	_, _ = io.WriteString(w, target)
	_, _ = w.Write([]byte{'\n'})
}

// sortedKeys returns the keys of a map in a stable order, so a digest does not
// depend on map iteration.
func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// VerifyFile checks that a copied file holds the same bytes as its source.
func VerifyFile(src, dst string) error {
	_, err := VerifyFileSummary(src, dst)
	return err
}

// VerifyFileSummary checks that a copied file holds the same bytes as its
// source, and returns the digest of the copy.
func VerifyFileSummary(src, dst string) (CopySummary, error) {
	srcInfo, err := os.Stat(src)
	if err != nil {
		return CopySummary{}, fmt.Errorf("verification failed: %w", err)
	}
	dstInfo, err := os.Stat(dst)
	if err != nil {
		return CopySummary{}, fmt.Errorf("verification failed: %w", err)
	}
	if dstInfo.Size() != srcInfo.Size() {
		return CopySummary{}, fmt.Errorf("verification failed: %s has %d bytes, the source file has %d", dst, dstInfo.Size(), srcInfo.Size())
	}
	srcSum, err := fileDigest(src)
	if err != nil {
		return CopySummary{}, fmt.Errorf("verification failed: %w", err)
	}
	dstSum, err := fileDigest(dst)
	if err != nil {
		return CopySummary{}, fmt.Errorf("verification failed: %w", err)
	}
	if !bytes.Equal(srcSum, dstSum) {
		return CopySummary{}, fmt.Errorf("verification failed: %s differs from the source file", dst)
	}
	return fileSummary(dstSum, dstInfo.Size()), nil
}

// fileSummary builds the digest of a single copied file.
func fileSummary(sum []byte, size int64) CopySummary {
	outer := sha256.New()
	writeFileEntry(outer, "", size, sum)
	return CopySummary{Digest: TreeDigest(hex.EncodeToString(outer.Sum(nil))), Files: 1, Bytes: size}
}

// DigestPath reads a stored copy and returns its digest: the digest of the file
// when path is a regular file, or of the whole tree when path is a folder.
// `exodrive verify` compares the result with the digest in the ledger.
func DigestPath(path string) (CopySummary, error) {
	info, err := os.Stat(path)
	if err != nil {
		return CopySummary{}, err
	}
	if info.Mode().IsRegular() {
		sum, err := fileDigest(path)
		if err != nil {
			return CopySummary{}, err
		}
		return fileSummary(sum, info.Size()), nil
	}
	if !info.IsDir() {
		return CopySummary{}, fmt.Errorf("check the copy: %s is neither a file nor a folder", path)
	}

	tree, err := scanTree(path)
	if err != nil {
		return CopySummary{}, err
	}
	outer := sha256.New()
	for _, rel := range sortedKeys(tree.sizes) {
		sum, err := fileDigest(filepath.Join(path, rel))
		if err != nil {
			return CopySummary{}, err
		}
		writeFileEntry(outer, rel, tree.sizes[rel], sum)
	}
	for _, rel := range sortedKeys(tree.links) {
		writeLinkEntry(outer, rel, tree.links[rel])
	}
	return CopySummary{
		Digest: TreeDigest(hex.EncodeToString(outer.Sum(nil))),
		Files:  tree.files,
		Bytes:  tree.bytes,
	}, nil
}

func fileDigest(path string) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()

	digest := sha256.New()
	buf := make([]byte, 1024*1024)
	if _, err := io.CopyBuffer(digest, file, buf); err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	return digest.Sum(nil), nil
}
