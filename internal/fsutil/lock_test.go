package fsutil

import "testing"

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
	names := []string{"npm", "cargo", "go"}

	cases := []struct {
		command string
		want    string
		matched bool
	}{
		{"/opt/homebrew/bin/cargo build", "cargo", true},
		{"node /usr/local/lib/node_modules/npm/bin/npm-cli.js install", "npm", true},
		{"/usr/local/go/bin/go test ./...", "go", true},
		{"/bin/zsh -c npm ci", "npm", true},
		{"/usr/bin/mdfind exodrive", "", false},
		{"exodrive migrate --tools=cargo", "", false},
		{"CARGO_HOME=/tmp/cargo ./exodrive migrate", "", false},
		{"GOFLAGS=-mod=mod go build ./...", "go", true},
		{"/usr/bin/gofmt -w .", "", false},
	}
	for _, tc := range cases {
		got, matched := matchProcess(tc.command, names)
		if got != tc.want || matched != tc.matched {
			t.Errorf("matchProcess(%q) = (%q, %v); want (%q, %v)", tc.command, got, matched, tc.want, tc.matched)
		}
	}
}

func TestSplitProcessLine(t *testing.T) {
	pid, command, ok := splitProcessLine("  421  node   /usr/local/bin/npm install ")
	if !ok || pid != "421" || command != "node   /usr/local/bin/npm install" {
		t.Fatalf("splitProcessLine = (%q, %q, %v)", pid, command, ok)
	}
	if _, _, ok := splitProcessLine("   "); ok {
		t.Fatal("splitProcessLine accepted a blank line")
	}
}
