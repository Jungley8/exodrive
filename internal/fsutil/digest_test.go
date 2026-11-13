//go:build unix

package fsutil

import (
	"os"
	"path/filepath"
	"testing"
)

// buildDigestTree writes a small tree that holds two files and a link.
func buildDigestTree(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "blobs"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "blobs", "a.bin"), []byte("first file"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "index.json"), []byte(`{"models":[]}`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("blobs/a.bin", filepath.Join(root, "latest")); err != nil {
		t.Fatal(err)
	}
	return root
}

// TestDigestPathMatchesTheVerifiedCopy keeps the two ways of reading a copy in
// step: the digest a migration records, and the digest a later check computes.
func TestDigestPathMatchesTheVerifiedCopy(t *testing.T) {
	src := buildDigestTree(t)
	dst := filepath.Join(t.TempDir(), "copy")
	if err := CopyDir(src, dst, 0, nil); err != nil {
		t.Fatalf("CopyDir failed: %v", err)
	}

	summary, err := VerifyTreeSummary(src, dst, nil)
	if err != nil {
		t.Fatalf("VerifyTreeSummary failed: %v", err)
	}
	if summary.Digest == "" {
		t.Fatal("the verified copy has no digest")
	}
	if summary.Files != 2 || summary.Bytes != int64(len("first file")+len(`{"models":[]}`)) {
		t.Fatalf("summary is %+v, want two files", summary)
	}

	again, err := DigestPath(dst)
	if err != nil {
		t.Fatalf("DigestPath failed: %v", err)
	}
	if again.Digest != summary.Digest {
		t.Fatalf("the digests differ: %s and %s", again.Digest, summary.Digest)
	}
	if again.Files != summary.Files || again.Bytes != summary.Bytes {
		t.Fatalf("the summary changed: %+v and %+v", again, summary)
	}

	// The same content under another path gives the same digest.
	elsewhere := filepath.Join(t.TempDir(), "again")
	if err := CopyDir(src, elsewhere, 0, nil); err != nil {
		t.Fatal(err)
	}
	moved, err := DigestPath(elsewhere)
	if err != nil {
		t.Fatal(err)
	}
	if moved.Digest != summary.Digest {
		t.Fatal("two copies of one tree report different digests")
	}
}

// TestDigestReportsADamagedCopy covers the cases a check must catch: one
// changed byte, an extra file, a missing file, and another link target.
func TestDigestReportsADamagedCopy(t *testing.T) {
	cases := []struct {
		name   string
		damage func(t *testing.T, dst string)
	}{
		{
			name: "changed byte",
			damage: func(t *testing.T, dst string) {
				if err := os.WriteFile(filepath.Join(dst, "blobs", "a.bin"), []byte("first filf"), 0644); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "extra file",
			damage: func(t *testing.T, dst string) {
				if err := os.WriteFile(filepath.Join(dst, "extra.bin"), []byte("x"), 0644); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "missing file",
			damage: func(t *testing.T, dst string) {
				if err := os.Remove(filepath.Join(dst, "index.json")); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "other link target",
			damage: func(t *testing.T, dst string) {
				if err := os.Remove(filepath.Join(dst, "latest")); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink("index.json", filepath.Join(dst, "latest")); err != nil {
					t.Fatal(err)
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src := buildDigestTree(t)
			dst := filepath.Join(t.TempDir(), "copy")
			if err := CopyDir(src, dst, 0, nil); err != nil {
				t.Fatal(err)
			}
			summary, err := VerifyTreeSummary(src, dst, nil)
			if err != nil {
				t.Fatalf("VerifyTreeSummary failed: %v", err)
			}

			tc.damage(t, dst)

			damaged, err := DigestPath(dst)
			if err != nil {
				t.Fatalf("DigestPath failed: %v", err)
			}
			if damaged.Digest == summary.Digest {
				t.Fatal("a damaged copy reports the digest of the good copy")
			}
		})
	}
}

// TestDigestOfASingleFileMatchesTheVerifiedCopy covers the Docker image case,
// where the copy is one file and not a tree.
func TestDigestOfASingleFileMatchesTheVerifiedCopy(t *testing.T) {
	src := filepath.Join(t.TempDir(), "Docker.raw")
	if err := os.WriteFile(src, []byte("image bytes"), 0644); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(t.TempDir(), "Docker.raw")
	if err := CopyFile(src, dst, 0, nil); err != nil {
		t.Fatal(err)
	}

	summary, err := VerifyFileSummary(src, dst)
	if err != nil {
		t.Fatalf("VerifyFileSummary failed: %v", err)
	}
	if summary.Files != 1 || summary.Bytes != int64(len("image bytes")) {
		t.Fatalf("summary is %+v, want one file", summary)
	}
	again, err := DigestPath(dst)
	if err != nil {
		t.Fatal(err)
	}
	if again.Digest != summary.Digest {
		t.Fatalf("the digests differ: %s and %s", again.Digest, summary.Digest)
	}

	if err := os.WriteFile(dst, []byte("image bytez"), 0644); err != nil {
		t.Fatal(err)
	}
	damaged, err := DigestPath(dst)
	if err != nil {
		t.Fatal(err)
	}
	if damaged.Digest == summary.Digest {
		t.Fatal("a changed file reports the digest of the good copy")
	}
}
