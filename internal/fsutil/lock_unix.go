//go:build unix

package fsutil

import (
	"os/exec"
	"strings"
)

// listProcesses reads the process table with ps. The three columns give the
// process ID, its parent, and the full command line.
func listProcesses() ([]processEntry, error) {
	out, err := exec.Command("ps", "-axo", "pid=,ppid=,args=").Output()
	if err != nil {
		return nil, err
	}

	var entries []processEntry
	for _, line := range strings.Split(string(out), "\n") {
		if pid, ppid, command, ok := splitProcessLine(line); ok {
			entries = append(entries, processEntry{pid: pid, ppid: ppid, command: command})
		}
	}
	return entries, nil
}

// splitProcessLine splits one `ps` line into the process ID, its parent, and
// the command. ps pads the columns with spaces, so each token is cut by hand.
func splitProcessLine(line string) (pid, ppid, command string, ok bool) {
	pid, rest := cutToken(strings.TrimSpace(line))
	if pid == "" {
		return "", "", "", false
	}
	ppid, rest = cutToken(rest)
	return pid, ppid, strings.TrimSpace(rest), true
}

// cutToken returns the first token of s and the text that follows it.
func cutToken(s string) (token, rest string) {
	s = strings.TrimLeft(s, " \t")
	if i := strings.IndexAny(s, " \t"); i >= 0 {
		return s[:i], s[i:]
	}
	return s, ""
}
