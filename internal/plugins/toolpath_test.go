package plugins

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestMergePathKeepsTheCallerOrder(t *testing.T) {
	got := MergePath("/usr/bin:/bin", "/opt/homebrew/bin:/usr/bin:/Users/me/.local/bin")
	want := "/usr/bin:/bin:/opt/homebrew/bin:/Users/me/.local/bin"
	if got != want {
		t.Fatalf("MergePath returned %q, want %q", got, want)
	}
}

func TestMergePathKeepsAnEmptyList(t *testing.T) {
	if got := MergePath("", "/opt/homebrew/bin"); got != "/opt/homebrew/bin" {
		t.Fatalf("MergePath returned %q, want the extra list", got)
	}
	if got := MergePath("/usr/bin", ""); got != "/usr/bin" {
		t.Fatalf("MergePath returned %q, want the current list", got)
	}
}

func TestLooksLikePathRejectsShellNoise(t *testing.T) {
	cases := []struct {
		line string
		want bool
	}{
		{"/opt/homebrew/bin:/usr/bin:/bin", true},
		{"Welcome to your shell", false},
		{"/usr/bin", false},
		{"~/bin:/usr/bin", false},
		{"", false},
	}
	for _, tc := range cases {
		if got := looksLikePath(tc.line); got != tc.want {
			t.Fatalf("looksLikePath(%q) = %v, want %v", tc.line, got, tc.want)
		}
	}
}

// A login shell prints other lines first. The query must read the PATH line,
// not the greeting.
func TestLoginShellPathReadsThePathLine(t *testing.T) {
	if _, err := os.Stat("/bin/sh"); err != nil {
		t.Skip("/bin/sh is unavailable")
	}
	dir := t.TempDir()
	shell := filepath.Join(dir, "shell")
	script := "#!/bin/sh\nprintf 'Welcome\\n'\nprintf '/opt/test/bin:/usr/bin:/bin\\n'\n"
	if err := os.WriteFile(shell, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SHELL", shell)

	got := loginShellPath(context.Background())
	if got != "/opt/test/bin:/usr/bin:/bin" {
		t.Fatalf("loginShellPath returned %q", got)
	}
}

func TestLoginShellPathIsEmptyWhenTheShellFails(t *testing.T) {
	if _, err := os.Stat("/bin/sh"); err != nil {
		t.Skip("/bin/sh is unavailable")
	}
	dir := t.TempDir()
	shell := filepath.Join(dir, "shell")
	if err := os.WriteFile(shell, []byte("#!/bin/sh\nexit 1\n"), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SHELL", shell)

	if got := loginShellPath(context.Background()); got != "" {
		t.Fatalf("loginShellPath returned %q, want empty", got)
	}
}
