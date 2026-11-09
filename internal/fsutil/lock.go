package fsutil

import (
	"bytes"
	"os/exec"
	"strings"
)

// ActiveProcess represents a running process detected by system inspection.
type ActiveProcess struct {
	Name string
	PID  string
}

// CheckRunningProcesses queries pgrep for any running processes in processNames.
func CheckRunningProcesses(processNames ...string) []ActiveProcess {
	var running []ActiveProcess
	for _, name := range processNames {
		cmd := exec.Command("pgrep", "-l", name)
		var out bytes.Buffer
		cmd.Stdout = &out
		if err := cmd.Run(); err == nil {
			lines := strings.Split(strings.TrimSpace(out.String()), "\n")
			for _, line := range lines {
				parts := strings.Fields(line)
				if len(parts) >= 2 {
					running = append(running, ActiveProcess{
						PID:  parts[0],
						Name: parts[1],
					})
				}
			}
		}
	}
	return running
}
