package desktop

import (
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/oneo-me/nsgo/nsgotest"

	exoCore "exodrive/internal/core"
)

// failingWindow mounts a window whose move always stops with a copy mismatch,
// and runs one move to the end. It returns the window and the place the log
// and the ledger live.
func failingWindow(t *testing.T) (*nsgotest.App, string) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv(exoCore.ManifestEnvVar, filepath.Join(dir, "manifest.json"))
	t.Setenv(exoCore.FailureLogEnvVar, "")

	engine := exoCore.NewEngine(&fakeProvider{
		name: "fake", items: []exoCore.CacheItem{fakeCache()},
		migrateErr: errors.New("verification failed: /tmp/copy differs from the source file"),
	})
	app := nsgotest.Start(t, NewMain(engine), nsgotest.Size(1100, 800))
	app.WaitUntil(2*time.Second, func() bool { return app.Exists("text:Fake Cache") })

	chooseTarget(t, app, t.TempDir())
	checkAll(t, app)
	app.Press("id:migrate")
	app.Settle()
	app.Press("id:confirm-move")
	app.WaitUntil(2*time.Second, func() bool { return app.Exists("id:retry-fake.cache") })
	return app, dir
}

func TestAFailedMoveLandsInTheLog(t *testing.T) {
	failingWindow(t)

	records := exoCore.RecentFailures(10)
	if len(records) != 1 {
		t.Fatalf("the log holds %d records, want 1: %+v", len(records), records)
	}
	got := records[0]
	if got.Action != actionMove || got.Kind != exoCore.KindVerification || got.Item != "fake.cache" {
		t.Fatalf("record = %+v, want a move failure of fake.cache", got)
	}
	if !strings.Contains(got.Message, "verification failed") {
		t.Fatalf("record message = %q, want the engine's reason", got.Message)
	}
}

func TestTheCardCopiesOneFailure(t *testing.T) {
	app, _ := failingWindow(t)

	app.Press("id:copy-fake.cache")
	app.Settle()

	clip := app.Clipboard
	if !strings.Contains(clip, "Exodrive") {
		t.Fatalf("the clipboard holds %q, want a report", clip)
	}
	if !strings.Contains(clip, "verification") {
		t.Fatalf("the clipboard holds %q, want the failure kind", clip)
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" && strings.Contains(clip, home) {
		t.Fatalf("the clipboard holds the home folder: %q", clip)
	}
	if !strings.Contains(app.Dump(), "Details copied") {
		t.Fatalf("the window does not say that it copied:\n%s", app.Dump())
	}
}

func TestTheReportShowsTheTextAndCopiesIt(t *testing.T) {
	app, _ := failingWindow(t)

	app.Press("id:report-open")
	app.WaitUntil(2*time.Second, func() bool { return app.Exists("id:report-text") })

	views := app.Query("id:report-text")
	if len(views) != 1 {
		t.Fatalf("the report holds %d text blocks, want 1", len(views))
	}
	shown := views[0].Text
	for _, want := range []string{"Exodrive", "move", "verification", "fake.cache"} {
		if !strings.Contains(shown, want) {
			t.Fatalf("the report text does not hold %q:\n%s", want, shown)
		}
	}

	app.Press("id:report-copy")
	app.Settle()
	if app.Clipboard != shown {
		t.Fatalf("the clipboard holds %q, want the text on screen %q", app.Clipboard, shown)
	}
}

func TestTheReportOpensAPrefilledIssue(t *testing.T) {
	app, _ := failingWindow(t)

	app.Press("id:report-open")
	app.WaitUntil(2*time.Second, func() bool { return app.Exists("id:report-text") })
	app.Press("id:report-issue")
	app.Settle()

	if len(app.OpenedURLs) != 1 {
		t.Fatalf("the window opened %d links, want 1", len(app.OpenedURLs))
	}
	link := app.OpenedURLs[0]
	if !strings.HasPrefix(link, IssuesURL+"?") {
		t.Fatalf("the window opened %q, want %s", link, IssuesURL)
	}
	u, err := url.Parse(link)
	if err != nil {
		t.Fatalf("the link does not parse: %v", err)
	}
	title := u.Query().Get("title")
	body := u.Query().Get("body")
	if !strings.Contains(title, "move") || !strings.Contains(title, "verification") {
		t.Fatalf("the issue title is %q, want the action and the kind", title)
	}
	if !strings.Contains(body, "fake.cache") {
		t.Fatalf("the issue body does not hold the cache: %q", body)
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" && strings.Contains(body, home) {
		t.Fatalf("the issue body holds the home folder: %q", body)
	}
}

func TestTheLogStaysQuietWhenOff(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(exoCore.ManifestEnvVar, filepath.Join(dir, "manifest.json"))
	t.Setenv(exoCore.FailureLogEnvVar, "1")

	engine := exoCore.NewEngine(&fakeProvider{
		name: "fake", items: []exoCore.CacheItem{fakeCache()},
		migrateErr: errors.New("verification failed: a byte differs"),
	})
	app := nsgotest.Start(t, NewMain(engine), nsgotest.Size(1100, 800))
	app.WaitUntil(2*time.Second, func() bool { return app.Exists("text:Fake Cache") })
	chooseTarget(t, app, t.TempDir())
	checkAll(t, app)
	app.Press("id:migrate")
	app.Settle()
	app.Press("id:confirm-move")
	app.WaitUntil(2*time.Second, func() bool { return app.Exists("id:retry-fake.cache") })

	if _, err := os.Stat(exoCore.FailurePath()); !os.IsNotExist(err) {
		t.Fatalf("the log exists while the switch is on: %v", err)
	}
	// The window still says what stopped, so the switch never hides a reason.
	if !strings.Contains(app.Dump(), "The copy did not match") {
		t.Fatalf("the window hid the reason while the log was off:\n%s", app.Dump())
	}
}
