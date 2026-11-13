package core

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// failureHome points the ledger and the failure log at a temporary folder, and
// leaves the log on.
func failureHome(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv(ManifestEnvVar, filepath.Join(dir, "manifest.json"))
	t.Setenv(FailureLogEnvVar, "")
	return dir
}

func readFailureLines(t *testing.T) []Failure {
	t.Helper()
	data, err := os.ReadFile(FailurePath())
	if err != nil {
		t.Fatalf("read the failure log: %v", err)
	}
	var out []Failure
	scan := bufio.NewScanner(bytes.NewReader(data))
	scan.Buffer(make([]byte, 0, 64<<10), 1<<20)
	for scan.Scan() {
		var f Failure
		if err := json.Unmarshal(scan.Bytes(), &f); err != nil {
			t.Fatalf("a line does not parse: %v: %s", err, scan.Text())
		}
		out = append(out, f)
	}
	return out
}

func TestRecordFailureWritesOneRecord(t *testing.T) {
	failureHome(t)

	err := RecordFailure(Failure{
		Version: "0.6.0",
		Action:  "move",
		Kind:    KindVerification,
		Item:    "npm.cache",
		Message: "verification failed: a file differs",
	})
	if err != nil {
		t.Fatalf("RecordFailure: %v", err)
	}

	records := readFailureLines(t)
	if len(records) != 1 {
		t.Fatalf("the log holds %d records, want 1", len(records))
	}
	got := records[0]
	if got.Version != "0.6.0" || got.Action != "move" || got.Kind != KindVerification || got.Item != "npm.cache" {
		t.Fatalf("record lost a field: %+v", got)
	}
	if got.OS == "" || got.Arch == "" {
		t.Fatalf("record holds no platform: %+v", got)
	}
	if _, err := time.Parse(time.RFC3339, got.Time); err != nil {
		t.Fatalf("time %q does not parse: %v", got.Time, err)
	}
}

func TestRecordFailureHidesTheHomeFolder(t *testing.T) {
	failureHome(t)

	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("home: %v", err)
	}
	if err := RecordFailure(Failure{Kind: KindSpace, Message: "insufficient space at " + filepath.Join(home, "Library", "Caches")}); err != nil {
		t.Fatalf("RecordFailure: %v", err)
	}

	records := readFailureLines(t)
	if strings.Contains(records[0].Message, home) {
		t.Fatalf("the record holds the home folder: %q", records[0].Message)
	}
	if !strings.HasPrefix(records[0].Message, "insufficient space at ~/") {
		t.Fatalf("message = %q, want a tilde path", records[0].Message)
	}
}

func TestRecordFailureCutsALongMessage(t *testing.T) {
	failureHome(t)

	msg := strings.Repeat("a", failureMessageLimit*3)
	if err := RecordFailure(Failure{Kind: KindOther, Message: msg}); err != nil {
		t.Fatalf("RecordFailure: %v", err)
	}

	records := readFailureLines(t)
	if got := len(records[0].Message); got > failureMessageLimit+len("…") {
		t.Fatalf("message holds %d bytes, want at most %d", got, failureMessageLimit)
	}
	if !strings.HasSuffix(records[0].Message, "…") {
		t.Fatalf("message = %q, want a cut mark", records[0].Message)
	}
}

func TestRecordFailureStaysQuietWhenOff(t *testing.T) {
	failureHome(t)
	t.Setenv(FailureLogEnvVar, "1")

	if err := RecordFailure(Failure{Kind: KindOther, Message: "anything"}); err != nil {
		t.Fatalf("RecordFailure: %v", err)
	}
	if _, err := os.Stat(FailurePath()); !os.IsNotExist(err) {
		t.Fatalf("the log exists while the switch is on: %v", err)
	}
}

func TestTheLogKeepsItsNewestRecords(t *testing.T) {
	dir := failureHome(t)

	// Fill the file past the cap with valid records that share a shape.
	var buf bytes.Buffer
	pad := strings.Repeat("x", 100)
	for i := 0; i < 4000; i++ {
		fmt.Fprintf(&buf, "{\"time\":\"2026-01-01T00:00:00Z\",\"kind\":\"other\",\"message\":\"%d-%s\"}\n", i, pad)
	}
	p := FailurePath()
	if err := os.WriteFile(filepath.Join(dir, "seed.tmp"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, buf.Bytes(), 0600); err != nil {
		t.Fatalf("seed the log: %v", err)
	}
	if info, _ := os.Stat(p); info.Size() <= failureLogLimit {
		t.Fatalf("the seed file holds %d bytes, want more than %d", info.Size(), failureLogLimit)
	}

	if err := RecordFailure(Failure{Kind: KindOther, Message: "the newest record"}); err != nil {
		t.Fatalf("RecordFailure: %v", err)
	}

	info, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() > failureLogLimit {
		t.Fatalf("the log holds %d bytes after a trim, want at most %d", info.Size(), failureLogLimit)
	}
	records := readFailureLines(t)
	last := records[len(records)-1]
	if last.Message != "the newest record" {
		t.Fatalf("last record = %q, want the new one", last.Message)
	}
	if strings.HasPrefix(records[0].Message, "0-") {
		t.Fatalf("the trim kept the oldest record")
	}
}

func TestRecentFailuresSkipsABrokenLine(t *testing.T) {
	failureHome(t)

	if err := RecordFailure(Failure{Kind: KindSpace, Message: "first"}); err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(FailurePath(), os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString("this is not json\n"); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()
	if err := RecordFailure(Failure{Kind: KindRunning, Message: "second"}); err != nil {
		t.Fatal(err)
	}

	records := RecentFailures(10)
	if len(records) != 2 {
		t.Fatalf("RecentFailures = %d records, want 2", len(records))
	}
	if records[0].Message != "first" || records[1].Message != "second" {
		t.Fatalf("RecentFailures = %q then %q, want oldest first", records[0].Message, records[1].Message)
	}
	if got := RecentFailures(1); len(got) != 1 || got[0].Message != "second" {
		t.Fatalf("RecentFailures(1) = %+v, want the newest record", got)
	}
}

func TestRecordFailureKeepsEveryConcurrentLine(t *testing.T) {
	failureHome(t)

	const writes = 20
	var wg sync.WaitGroup
	errs := make(chan error, writes)
	for i := 0; i < writes; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errs <- RecordFailure(Failure{Kind: KindOther, Message: fmt.Sprintf("record %d", i)})
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("RecordFailure: %v", err)
		}
	}

	if got := len(readFailureLines(t)); got != writes {
		t.Fatalf("the log holds %d records, want %d", got, writes)
	}
}

func TestRecentFailuresReadsAMissingLogAsNoRecord(t *testing.T) {
	failureHome(t)

	if got := RecentFailures(5); len(got) != 0 {
		t.Fatalf("RecentFailures = %+v, want no record", got)
	}
}
