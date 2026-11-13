package plugins

import (
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// pathOnce keeps the login shell query to one run per process.
var pathOnce sync.Once

// UseLoginShellPath adds the folders of the user's login shell to PATH.
//
// A window opened from Finder or the Dock starts with the launchd PATH, which
// holds only /usr/bin, /bin, /usr/sbin, and /sbin. Homebrew, nvm, and custom
// folders are missing, so every tool query fails and each cache shows Blocked.
// The user's terminal finds the same tools, because the shell sets PATH.
// A Windows window inherits the PATH of the user, so it runs no query.
//
// The command line already inherits the shell PATH, so a run that finds its
// tools never pays for the query.
func UseLoginShellPath(ctx context.Context) {
	pathOnce.Do(func() {
		login := loginShellPath(ctx)
		if login == "" {
			return
		}
		_ = os.Setenv("PATH", MergePath(os.Getenv("PATH"), login))
	})
}

// MergePath appends the folders that the first list lacks, in order. An
// existing folder keeps its place, so the caller's order wins.
func MergePath(currentPath, extraPath string) string {
	seen := make(map[string]bool)
	var merged []string
	for _, list := range []string{currentPath, extraPath} {
		for _, entry := range strings.Split(list, string(os.PathListSeparator)) {
			if entry == "" || seen[entry] {
				continue
			}
			seen[entry] = true
			merged = append(merged, entry)
		}
	}
	return strings.Join(merged, string(os.PathListSeparator))
}

// loginShellPath asks the login shell for its PATH. An unreadable answer is
// empty, and the process keeps its own PATH.
func loginShellPath(ctx context.Context) string {
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = defaultShell()
	}
	if shell == "" {
		return ""
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, shell, "-lic", `printf '%s\n' "$PATH"`)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = io.Discard
	if err := cmd.Run(); err != nil {
		return ""
	}

	// An interactive shell may print other lines. The PATH line holds absolute
	// folders and separators only, so read from the last line back.
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if looksLikePath(line) {
			return line
		}
	}
	return ""
}

func looksLikePath(line string) bool {
	entries := strings.Split(line, string(os.PathListSeparator))
	if len(entries) < 2 {
		return false
	}
	for _, entry := range entries {
		if !strings.HasPrefix(entry, "/") {
			return false
		}
	}
	return true
}
