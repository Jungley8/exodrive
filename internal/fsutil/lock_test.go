package fsutil

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func TestProcessBaseMatches(t *testing.T) {
	cases := []struct {
		base, name string
		want       bool
	}{
		{"npm", "npm", true},
		{"npm-cli.js", "npm", true},
		{"pnpm.cjs", "pnpm", true},
		{"yarn.js", "yarn", true},
		{"cargo", "cargo", true},
		{"Docker", "Docker", true},
		{"gofmt", "go", false},
		{"gopls", "go", false},
		{"npm-extra.js", "npm", false},
		{"my-npm", "npm", false},
		{"node", "npm", false},
		// A Java main class names its tool in the last segment.
		{"org.gradle.launcher.daemon.bootstrap.GradleDaemon", "GradleDaemon", true},
		{"org.gradle.wrapper.GradleWrapperMain", "GradleWrapperMain", true},
		// A data file is not a program. Only a capital segment counts.
		{"notes.go", "go", false},
		{"x.mvn", "mvn", false},
	}
	for _, tc := range cases {
		if got := processBaseMatches(tc.base, tc.name); got != tc.want {
			t.Errorf("processBaseMatches(%q, %q) = %v; want %v", tc.base, tc.name, got, tc.want)
		}
	}
}

func TestMatchProcess(t *testing.T) {
	names := []string{"npm", "cargo", "go", "dart", "pip", "GradleDaemon"}

	cases := []struct {
		command string
		want    string
		matched bool
	}{
		{"/opt/homebrew/bin/cargo build", "cargo", true},
		{"node /usr/local/lib/node_modules/npm/bin/npm-cli.js install", "npm", true},
		{"/usr/local/go/bin/go test ./...", "go", true},
		// A sibling shell names the tool as text. The tool process itself
		// carries the name, so the lock still finds a running npm.
		{"/bin/zsh -c npm ci", "", false},
		{"/usr/bin/mdfind exodrive", "", false},
		{"exodrive migrate --tools=cargo", "", false},
		{"CARGO_HOME=/tmp/cargo ./exodrive migrate", "", false},
		{"GOFLAGS=-mod=mod go build ./...", "go", true},
		{"/usr/bin/gofmt -w .", "", false},
		// A tool name as an argument starts no tool.
		{"grep -i dart /tmp/exodrive.log", "", false},
		{"sh -c '/tmp/fixture/bin/conda info --json'", "", false},
		// A module after -m runs inside the interpreter.
		{"python3 -m pip install requests", "pip", true},
		// A Java main class names its tool.
		{"java -cp /tmp/x org.gradle.launcher.daemon.bootstrap.GradleDaemon", "GradleDaemon", true},
	}
	for _, tc := range cases {
		got, matched := matchProcess(tc.command, names)
		if got != tc.want || matched != tc.matched {
			t.Errorf("matchProcess(%q) = (%q, %v); want (%q, %v)", tc.command, got, matched, tc.want, tc.matched)
		}
	}
}

// TestAncestors skips the shell above Exodrive, so its command line cannot look
// like a running tool.
func TestAncestors(t *testing.T) {
	entries := []processEntry{
		{pid: "100", ppid: "50"},
		{pid: "50", ppid: "1"},
		{pid: "1", ppid: "0"},
		{pid: "200", ppid: "1"},
		{pid: "300", ppid: "300"},
	}

	above := ancestors(entries, "100")
	for _, want := range []string{"50", "1"} {
		if !above[want] {
			t.Fatalf("ancestors skips %s: %v", want, above)
		}
	}
	for _, unwanted := range []string{"100", "200", "300", "0"} {
		if above[unwanted] {
			t.Fatalf("ancestors holds %s: %v", unwanted, above)
		}
	}
	if len(above) != 2 {
		t.Fatalf("ancestors holds %d entries: %v", len(above), above)
	}

	// A table that does not hold the start process yields no ancestors, and a
	// loop ends.
	if got := ancestors(entries, "999"); len(got) != 0 {
		t.Fatalf("ancestors invented %v", got)
	}
	if got := ancestors(entries, "300"); len(got) != 0 {
		t.Fatalf("ancestors looped: %v", got)
	}
}

func TestSplitProcessLine(t *testing.T) {
	pid, ppid, command, ok := splitProcessLine("  421    1  node   /usr/local/bin/npm install ")
	if !ok || pid != "421" || ppid != "1" || command != "node   /usr/local/bin/npm install" {
		t.Fatalf("splitProcessLine = (%q, %q, %q, %v)", pid, ppid, command, ok)
	}
	if _, _, _, ok := splitProcessLine("   "); ok {
		t.Fatal("splitProcessLine accepted a blank line")
	}
}

// mockProcessTable points the process table at a script that prints rows, so
// a test can decide what runs on the machine.
func mockProcessTable(t *testing.T, rows string) {
	t.Helper()
	dir := t.TempDir()
	script := "#!/bin/sh\nprintf '%s\\n' " + rows + "\n"
	if err := os.WriteFile(filepath.Join(dir, "ps"), []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// TestCheckRunningProcessesSkipsItsOwnShell keeps the shell that started
// Exodrive out of the answer, even when its command line names the tool.
func TestCheckRunningProcessesSkipsItsOwnShell(t *testing.T) {
	self := strconv.Itoa(os.Getpid())
	parent := strconv.Itoa(os.Getppid())
	mockProcessTable(t, `"`+self+` `+parent+` exodrive migrate --tools=dart" "`+
		parent+` 1 /bin/zsh -c /opt/homebrew/bin/dart run build"`)

	running, err := CheckRunningProcesses("dart")
	if err != nil {
		t.Fatal(err)
	}
	if len(running) != 0 {
		t.Fatalf("the check blamed a process above Exodrive: %+v", running)
	}
}

// TestCheckRunningProcessesReportsAnUnrelatedProcess proves that the skip only
// covers the processes above Exodrive.
func TestCheckRunningProcessesReportsAnUnrelatedProcess(t *testing.T) {
	self := strconv.Itoa(os.Getpid())
	mockProcessTable(t, `"`+self+` 1 exodrive migrate" "4242 1 /opt/homebrew/bin/dart run build"`)

	running, err := CheckRunningProcesses("dart")
	if err != nil {
		t.Fatal(err)
	}
	if len(running) != 1 || running[0].PID != "4242" {
		t.Fatalf("the check missed a running tool: %+v", running)
	}
}
