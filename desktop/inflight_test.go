package desktop

import (
	"strings"
	"testing"
	"time"

	"github.com/oneo-me/nsgo/nsgotest"

	exoCore "exodrive/internal/core"
)

// A stop in the middle of a move leaves a record, and the next start names the
// cache that was changing.
func TestWindowReportsTheMoveThatTheLastRunLeft(t *testing.T) {
	ledgerPath(t)
	if err := exoCore.WriteInFlight(exoCore.InFlight{ID: "fake.cache", Title: "Fake Cache", Action: actionMove}); err != nil {
		t.Fatal(err)
	}

	app := nsgotest.Start(t, NewMain(newFakeEngine(fakeCache())), nsgotest.Size(1100, 800))
	app.WaitUntil(2*time.Second, func() bool {
		return strings.Contains(app.Dump(), "The window closed during a move")
	})
	if !strings.Contains(app.Dump(), "Fake Cache was moving when Exodrive stopped.") {
		t.Fatalf("the notice does not name the cache:\n%s", app.Dump())
	}
	if _, ok := exoCore.ReadInFlight(); ok {
		t.Fatal("the record survived the start that read it")
	}
}

// The window records the move that runs, so a stop leaves a reason.
func TestWindowRecordsTheMoveThatRuns(t *testing.T) {
	ledgerPath(t)

	hold := make(chan struct{})
	engine := exoCore.NewEngine(&fakeProvider{
		name: "fake", items: []exoCore.CacheItem{fakeCache()}, hold: hold,
	})
	app := nsgotest.Start(t, NewMain(engine), nsgotest.Size(1100, 800))
	app.WaitUntil(2*time.Second, func() bool { return app.Exists("text:Fake Cache") })

	chooseTarget(t, app, t.TempDir())
	checkAll(t, app)
	app.Press("id:migrate")
	app.Settle()
	app.Press("id:confirm-move")

	app.WaitUntil(2*time.Second, func() bool { _, ok := exoCore.ReadInFlight(); return ok })
	if got, _ := exoCore.ReadInFlight(); got.ID != "fake.cache" || got.Action != actionMove {
		t.Fatalf("the record reads %+v", got)
	}

	close(hold)
	app.WaitUntil(3*time.Second, func() bool { _, ok := exoCore.ReadInFlight(); return !ok })
}
