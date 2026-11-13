package core

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
	"unicode/utf8"
)

// FailureLogEnvVar turns the failure log off. Any non-empty value stops a
// write. A person sets it when they want nothing on disk.
const FailureLogEnvVar = "EXODRIVE_NO_FAILURE_LOG"

const (
	// failureLogLimit caps the log. A line holds a short message, so the cap
	// keeps about 300 to 600 records, and one more write trims it to half.
	failureLogLimit = 256 << 10
	// failureMessageLimit caps one message, in bytes. A path can be long, and
	// a tool can print a long reason.
	failureMessageLimit = 600
)

// Failure is one record in the failure log. It holds no secret: the message is
// redacted before it reaches the disk.
type Failure struct {
	Time    string `json:"time"`
	Version string `json:"version"`
	OS      string `json:"os"`
	Arch    string `json:"arch"`
	Action  string `json:"action"`
	Kind    string `json:"kind"`
	Item    string `json:"item,omitempty"`
	Message string `json:"message,omitempty"`
}

// FailurePath returns the failure log. It sits beside the ledger, so a run
// that moves the ledger moves the log with it.
func FailurePath() string { return StatePath("failures.jsonl") }

// RecordFailure appends one record. It is best effort: a caller ignores the
// error, so a full disk or a read-only folder never changes the result of a
// move, a move back, or a delete.
func RecordFailure(f Failure) error {
	if strings.TrimSpace(os.Getenv(FailureLogEnvVar)) != "" {
		return nil
	}
	if f.Time == "" {
		f.Time = time.Now().UTC().Format(time.RFC3339)
	}
	if f.OS == "" {
		f.OS = runtime.GOOS
	}
	if f.Arch == "" {
		f.Arch = runtime.GOARCH
	}
	f.Message = CutMessage(Redact(f.Message))

	line, err := json.Marshal(f)
	if err != nil {
		return err
	}
	line = append(line, '\n')

	unlock, err := lockPath(FailurePath())
	if err != nil {
		return err
	}
	defer unlock()

	p := FailurePath()
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		return err
	}
	fh, err := os.OpenFile(p, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return err
	}
	if _, err := fh.Write(line); err != nil {
		_ = fh.Close()
		return err
	}
	if err := fh.Close(); err != nil {
		return err
	}
	return trimFailureLog(p)
}

// Redact hides the home folder behind a tilde. A username sits inside that
// path, so a record never carries it. When the home folder is unknown, the
// text is dropped: a record with no message still names what stopped.
func Redact(text string) string {
	if text == "" {
		return ""
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	return strings.ReplaceAll(text, home, "~")
}

// CutMessage limits a message to failureMessageLimit bytes and keeps the text
// valid UTF-8, so a long path from a tool cannot fill the log.
func CutMessage(s string) string {
	if len(s) <= failureMessageLimit {
		return s
	}
	b := []byte(s)[:failureMessageLimit]
	for len(b) > 0 && !utf8.Valid(b) {
		b = b[:len(b)-1]
	}
	return string(b) + "…"
}

// trimFailureLog keeps the newest whole lines when the log passes its cap. The
// caller holds the lock, so a second process never appends to the old file.
func trimFailureLog(p string) error {
	info, err := os.Stat(p)
	if err != nil {
		return err
	}
	if info.Size() <= failureLogLimit {
		return nil
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return err
	}

	keep := failureLogLimit / 2
	start := len(data) - keep
	if start < 0 {
		start = 0
	}
	if i := bytes.IndexByte(data[start:], '\n'); i >= 0 {
		start += i + 1
	} else {
		start = len(data)
	}
	trimmed := data[start:]

	dir := filepath.Dir(p)
	tmp, err := os.CreateTemp(dir, filepath.Base(p)+".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	cleanup := func() { _ = os.Remove(tmpName) }

	if _, err := tmp.Write(trimmed); err != nil {
		_ = tmp.Close()
		cleanup()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		cleanup()
		return err
	}
	if err := tmp.Chmod(0600); err != nil {
		_ = tmp.Close()
		cleanup()
		return err
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return err
	}
	if err := os.Rename(tmpName, p); err != nil {
		cleanup()
		return err
	}
	return nil
}

// RecentFailures reads the newest n records, oldest first. A line that does
// not parse is skipped, and a missing file reads as no record: reading the log
// never fails a caller.
func RecentFailures(n int) []Failure {
	if n <= 0 {
		return nil
	}
	data, err := os.ReadFile(FailurePath())
	if err != nil {
		return nil
	}
	lines := bytes.Split(bytes.TrimRight(data, "\n"), []byte("\n"))
	out := make([]Failure, 0, n)
	for i := len(lines) - 1; i >= 0 && len(out) < n; i-- {
		if len(bytes.TrimSpace(lines[i])) == 0 {
			continue
		}
		var f Failure
		if json.Unmarshal(lines[i], &f) != nil {
			continue
		}
		out = append(out, f)
	}
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out
}
