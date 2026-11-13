package desktop

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/oneo-me/nsgo/nsgotest"

	exoCore "exodrive/internal/core"
)

// ledgerPath points the ledger at a temporary file and returns its folder, so
// the welcome marker lands there too.
func ledgerPath(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv(exoCore.ManifestEnvVar, filepath.Join(dir, "manifest.json"))
	return dir
}

// The first run names the three steps and offers the first one.
func TestWindowWelcomesTheFirstRun(t *testing.T) {
	ledgerPath(t)

	app := nsgotest.Start(t, NewMain(newFakeEngine(fakeCache())), nsgotest.Size(1100, 800))
	app.WaitUntil(2*time.Second, func() bool { return app.Exists("text:Fake Cache") })
	app.WaitUntil(2*time.Second, func() bool { return app.Exists("id:welcome-start") })

	if !strings.Contains(app.Dump(), "Three steps move a cache") {
		t.Fatalf("the first run shows no welcome:\n%s", app.Dump())
	}
	for _, step := range []string{
		"Choose a folder on the external drive.",
		"Select the caches to move.",
		"Press Move. A cache frees its space after the copy passes the check.",
	} {
		if !strings.Contains(app.Dump(), step) {
			t.Fatalf("the welcome omits %q:\n%s", step, app.Dump())
		}
	}
}

// Skip hides the card, and the next start opens on the work.
func TestWindowKeepsTheWelcomeAwayAfterSkip(t *testing.T) {
	dir := ledgerPath(t)

	app := nsgotest.Start(t, NewMain(newFakeEngine(fakeCache())), nsgotest.Size(1100, 800))
	app.WaitUntil(2*time.Second, func() bool { return app.Exists("id:welcome-skip") })
	app.Press("id:welcome-skip")
	app.Settle()
	if app.Exists("id:welcome-start") {
		t.Fatalf("Skip left the welcome open:\n%s", app.Dump())
	}
	if _, err := os.Stat(filepath.Join(dir, welcomeFile)); err != nil {
		t.Fatalf("Skip recorded nothing: %v", err)
	}

	again := nsgotest.Start(t, NewMain(newFakeEngine(fakeCache())), nsgotest.Size(1100, 800))
	again.WaitUntil(2*time.Second, func() bool { return again.Exists("text:Fake Cache") })
	if again.Exists("id:welcome-start") {
		t.Fatalf("the second start showed the welcome again:\n%s", again.Dump())
	}
}

// Choosing a folder is the first step, so it closes the card.
func TestWindowClosesTheWelcomeWhenAFolderIsChosen(t *testing.T) {
	ledgerPath(t)

	app := nsgotest.Start(t, NewMain(newFakeEngine(fakeCache())), nsgotest.Size(1100, 800))
	app.WaitUntil(2*time.Second, func() bool { return app.Exists("id:welcome-start") })

	chooseTarget(t, app, t.TempDir())
	app.Settle()
	if app.Exists("id:welcome-start") {
		t.Fatalf("a chosen folder left the welcome open:\n%s", app.Dump())
	}
}

// A card that was never marked costs one press, never a failed start.
func TestWelcomeSeenWithoutAMarker(t *testing.T) {
	ledgerPath(t)
	if welcomeSeen() {
		t.Fatal("a fresh folder counts as a seen welcome")
	}
	markWelcomeSeen()
	if !welcomeSeen() {
		t.Fatal("the marker did not record the welcome")
	}
}
