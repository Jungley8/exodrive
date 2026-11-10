package ui

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"

	"exodrive/internal/core"
)

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		bytes    int64
		expected string
	}{
		{0, "0 B"},
		{500, "500 B"},
		{1023, "1023 B"},
		{1024, "1.0 KB"},
		{1536, "1.5 KB"},
		{1048576, "1.0 MB"},
		{1073741824, "1.0 GB"},
		{1099511627776, "1.0 TB"},
	}

	for _, tt := range tests {
		got := FormatBytes(tt.bytes)
		if got != tt.expected {
			t.Errorf("FormatBytes(%d) = %q, want %q", tt.bytes, got, tt.expected)
		}
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		input    string
		maxLen   int
		expected string
	}{
		{"hello", 10, "hello"},
		{"hello", 5, "hello"},
		{"hello world", 8, "hello .."},
		{"abcdef", 4, "ab.."},
	}

	for _, tt := range tests {
		got := truncate(tt.input, tt.maxLen)
		if got != tt.expected {
			t.Errorf("truncate(%q, %d) = %q, want %q", tt.input, tt.maxLen, got, tt.expected)
		}
	}
}

func TestRenderTable(t *testing.T) {
	items := []core.CacheItem{
		{
			ID:          "test.item",
			Title:       "Test Item",
			Category:    core.CategoryPackageStore,
			SizeBytes:   1048576,
			IsMigrated:  false,
			IsActive:    true,
			CurrentPath: "/path/to/test",
			Warning:     "Warning message",
		},
		{
			ID:          "test.migrated",
			Title:       "Migrated Item",
			Category:    core.CategoryBuildCache,
			SizeBytes:   2097152,
			IsMigrated:  true,
			IsActive:    true,
			CurrentPath: "/path/to/migrated",
		},
		{
			ID:          "test.abandoned",
			Title:       "Abandoned Item",
			Category:    core.CategoryOrphan,
			SizeBytes:   512,
			IsMigrated:  false,
			IsActive:    false,
			CurrentPath: "/path/to/abandoned",
		},
	}

	// Capture stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	RenderTable(items)

	_ = w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	output := buf.String()

	if !strings.Contains(output, "TOOL / CACHE") {
		t.Errorf("expected table header in output: %s", output)
	}
	if !strings.Contains(output, "Test Item") {
		t.Errorf("expected item title in output: %s", output)
	}
	if !strings.Contains(output, "Warning message") {
		t.Errorf("expected warning in output: %s", output)
	}
}

func TestRenderProgressBar(t *testing.T) {
	// Total <= 0 should do nothing
	RenderProgressBar("Copying", 0, 0)
	RenderProgressBar("Copying", 50, 100)
	RenderProgressBar("Copying", 150, 100) // percent > 100
}

// The table says how Exodrive points each tool at the copy, so a user can tell
// a tool setting from a symbolic link before the move.
func TestRenderTableShowsTheSwitchMethod(t *testing.T) {
	items := []core.CacheItem{
		{ID: "npm.cache", ToolName: "npm", Title: "NPM Cache", Category: core.CategoryPackageStore, CurrentPath: "/Users/x/.npm", SizeBytes: 10, IsActive: true, RedirectMethod: core.RedirectNativeConfig},
		{ID: "cargo.registry", ToolName: "cargo", Title: "Cargo Registry", Category: core.CategoryPackageStore, CurrentPath: "/Users/x/.cargo/registry", SizeBytes: 20, IsActive: true, RedirectMethod: core.RedirectSymlink},
		{ID: "orphan.pnpm", ToolName: "pnpm", Title: "Old PNPM", Category: core.CategoryOrphan, CurrentPath: "/Users/x/Library/Caches/pnpm", SizeBytes: 30, RedirectMethod: core.RedirectManual},
	}

	out := captureStdout(t, func() { RenderTable(items) })

	for _, want := range []string{"METHOD", "config", "symlink", "manual"} {
		if !strings.Contains(out, want) {
			t.Fatalf("the table does not show %q:\n%s", want, out)
		}
	}
}

// captureStdout runs fn and returns what it printed to stdout.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	fn()
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	os.Stdout = old
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}
