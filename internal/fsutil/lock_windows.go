//go:build windows

package fsutil

import (
	"encoding/csv"
	"os/exec"
	"strings"
)

// listProcesses reads the process table with PowerShell. Win32_Process holds
// the image name, its parent, and the full command line, which is the same
// shape that `ps -axo pid=,ppid=,args=` gives on macOS and Linux. The command
// line decides the answer: npm and pnpm run through node.exe.
func listProcesses() ([]processEntry, error) {
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command",
		"Get-CimInstance Win32_Process | Select-Object ProcessId,ParentProcessId,CommandLine | ConvertTo-Csv -NoTypeInformation")
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	records, err := csv.NewReader(strings.NewReader(string(out))).ReadAll()
	if err != nil {
		return nil, err
	}

	entries := make([]processEntry, 0, len(records))
	for i, record := range records {
		if len(record) < 3 {
			continue
		}
		if i == 0 && strings.EqualFold(strings.TrimSpace(record[0]), "ProcessId") {
			continue
		}
		command := strings.TrimSpace(record[2])
		if command == "" {
			continue
		}
		entries = append(entries, processEntry{
			pid:     strings.TrimSpace(record[0]),
			ppid:    strings.TrimSpace(record[1]),
			command: command,
		})
	}
	return entries, nil
}
