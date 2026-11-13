//go:build unix

package fsutil

import (
	"os/exec"
	"strings"
)

// listProcesses reads the process table with ps. The two columns give the
// process ID and the full command line.
func listProcesses() ([]processEntry, error) {
	out, err := exec.Command("ps", "-axo", "pid=,args=").Output()
	if err != nil {
		return nil, err
	}

	var entries []processEntry
	for _, line := range strings.Split(string(out), "\n") {
		if pid, command, ok := splitProcessLine(line); ok {
			entries = append(entries, processEntry{pid: pid, command: command})
		}
	}
	return entries, nil
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
