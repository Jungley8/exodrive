package fsutil

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// ActiveProcess represents a running process detected by system inspection.
type ActiveProcess struct {
	Name string
	PID  string
}

// CheckRunningProcesses reads the process table and reports the processes that
// run one of processNames. It matches the executable and the arguments, so a
// tool that starts through an interpreter is still found: npm runs as
// `node .../npm-cli.js`. An unreadable process table is an error, because a
// migration must never run without the check.
func CheckRunningProcesses(processNames ...string) ([]ActiveProcess, error) {
	if len(processNames) == 0 {
		return nil, nil
	}

	cmd := exec.Command("ps", "-axo", "pid=,args=")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("read the process table: %w", err)
	}

	self := strconv.Itoa(os.Getpid())
	var running []ActiveProcess
	for _, line := range strings.Split(string(out), "\n") {
		pid, command, ok := splitProcessLine(line)
		if !ok || pid == self {
			continue
		}
		if name, matched := matchProcess(command, processNames); matched {
			running = append(running, ActiveProcess{PID: pid, Name: name})
		}
	}
	return running, nil
}

// splitProcessLine splits one `ps` line into the process ID and its command.
func splitProcessLine(line string) (pid, command string, ok bool) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return "", "", false
	}
	idx := strings.IndexAny(trimmed, " \t")
	if idx < 0 {
		return trimmed, "", true
	}
	return trimmed[:idx], strings.TrimSpace(trimmed[idx+1:]), true
}

// matchProcess returns the tool name that command runs, if any. It looks at
// every token: the executable, an argument, or a path inside a shell command.
// A flag and a shell assignment are skipped, so `--tools=cargo` and
// `CARGO_HOME=/tmp/cargo exodrive migrate` do not look like the cargo tool.
func matchProcess(command string, names []string) (string, bool) {
	for _, field := range strings.Fields(command) {
		token := strings.Trim(field, "\"'")
		if strings.HasPrefix(token, "-") || isAssignment(token) {
			continue
		}
		base := filepath.Base(token)
		for _, name := range names {
			if processBaseMatches(base, name) {
				return name, true
			}
		}
	}
	return "", false
}

// isAssignment reports whether token is a shell assignment such as
// CARGO_HOME=/tmp/cargo. Its value is data, not a command.
func isAssignment(token string) bool {
	name, _, ok := strings.Cut(token, "=")
	if !ok || name == "" {
		return false
	}
	for i, r := range name {
		switch {
		case r == '_':
		case r >= 'A' && r <= 'Z':
		case r >= 'a' && r <= 'z':
		case i > 0 && r >= '0' && r <= '9':
		default:
			return false
		}
	}
	return true
}

// processBaseMatches reports whether base is the tool's executable or a script
// that belongs to it, such as npm-cli.js, pnpm.cjs, or yarn.js.
func processBaseMatches(base, name string) bool {
	if base == name {
		return true
	}
	if !strings.HasPrefix(base, name) {
		return false
	}
	switch strings.TrimPrefix(base, name) {
	case ".js", ".cjs", ".mjs", ".exe", "-cli.js", "-cli.cjs", "-cli.mjs":
		return true
	default:
		return false
	}
}
