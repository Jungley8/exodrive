package fsutil

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// ActiveProcess represents a running process detected by system inspection.
type ActiveProcess struct {
	Name string
	PID  string
}

// processEntry is one row of the process table: its ID, its parent, and its
// full command.
type processEntry struct {
	pid     string
	ppid    string
	command string
}

// CheckRunningProcesses reads the process table and reports the processes that
// run one of processNames. It matches the executable and a path or module that
// names the tool, so a tool that starts through an interpreter is still found:
// npm runs as `node .../npm-cli.js`, and pip runs as `python -m pip`. A process
// above Exodrive is skipped, because a shell that mentions a tool starts no
// tool. An unreadable process table is an error, because a migration must never
// run without the check.
func CheckRunningProcesses(processNames ...string) ([]ActiveProcess, error) {
	if len(processNames) == 0 {
		return nil, nil
	}

	entries, err := listProcesses()
	if err != nil {
		return nil, fmt.Errorf("read the process table: %w", err)
	}

	self := strconv.Itoa(os.Getpid())
	above := ancestors(entries, self)
	var running []ActiveProcess
	for _, entry := range entries {
		if entry.pid == self || above[entry.pid] {
			continue
		}
		if name, matched := matchProcess(entry.command, processNames); matched {
			running = append(running, ActiveProcess{PID: entry.pid, Name: name})
		}
	}
	return running, nil
}

// ancestors returns the process IDs above start. Exodrive runs under a shell or
// a make target whose command line can hold the tool name as text, and that
// process is not the tool.
func ancestors(entries []processEntry, start string) map[string]bool {
	parent := make(map[string]string, len(entries))
	for _, entry := range entries {
		parent[entry.pid] = entry.ppid
	}
	above := make(map[string]bool, 8)
	pid := start
	for i := 0; i < 64; i++ {
		up, ok := parent[pid]
		if !ok || up == "" || up == "0" || up == pid || above[up] {
			return above
		}
		above[up] = true
		pid = up
	}
	return above
}

// matchProcess returns the tool name that command runs, if any. The first token
// that is neither a flag nor a shell assignment is the program. A later token
// names a program only when it is a path, a Java class, or a module after -m:
// `grep dart` starts no dart, `node .../npm-cli.js` does, and so does
// `java -cp ... org.gradle.launcher.daemon.bootstrap.GradleDaemon`.
func matchProcess(command string, names []string) (string, bool) {
	fields := strings.Fields(command)
	programSeen := false
	for i, field := range fields {
		token := strings.Trim(field, "\"'")
		if token == "" || strings.HasPrefix(token, "-") || isAssignment(token) {
			continue
		}
		switch {
		case !programSeen:
			programSeen = true
		case !namesProgram(strings.Trim(fields[i-1], "\"'"), token):
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

// namesProgram reports whether a token after the program names another program
// rather than an argument: a path, a Java main class, or a module after -m.
func namesProgram(previous, token string) bool {
	if strings.ContainsRune(token, filepath.Separator) {
		return true
	}
	if previous == "-m" || previous == "--module" {
		return true
	}
	if i := strings.LastIndex(token, "."); i >= 0 && i+1 < len(token) {
		c := token[i+1]
		return c >= 'A' && c <= 'Z'
	}
	return false
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
//
// A Java main class names its tool too: the Gradle daemon runs as
// org.gradle.launcher.daemon.bootstrap.GradleDaemon. The rule wants a capital
// letter in the last segment, so a path such as notes.go is not the go tool.
func processBaseMatches(base, name string) bool {
	if base == name {
		return true
	}
	if i := strings.LastIndex(base, "."); i >= 0 && i+1 < len(base) {
		if c := base[i+1]; c >= 'A' && c <= 'Z' && base[i+1:] == name {
			return true
		}
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
