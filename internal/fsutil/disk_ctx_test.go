package fsutil

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestDirSizeStopsOnCancelledContext(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "file"), []byte("data"), 0644); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := DirSize(ctx, dir); !errors.Is(err, context.Canceled) {
		t.Fatalf("DirSize on a cancelled context returned %v, want context.Canceled", err)
	}
}

func TestDirSizeMeasuresThroughSymlinkedRoot(t *testing.T) {
	root := t.TempDir()
	data := filepath.Join(root, "store")
	if err := os.MkdirAll(data, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(data, "entry"), make([]byte, 2048), 0644); err != nil {
		t.Fatal(err)
	}

	link := filepath.Join(t.TempDir(), "linked-store")
	if err := os.Symlink(data, link); err != nil {
		t.Fatal(err)
	}

	size, err := DirSize(context.Background(), link)
	if err != nil {
		t.Fatalf("DirSize failed: %v", err)
	}
	if size != 2048 {
		t.Fatalf("DirSize through a symlink root = %d, want 2048", size)
	}
}
