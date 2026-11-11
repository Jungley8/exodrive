package desktop

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/oneo-me/nsgo/nsgotest"

	exoCore "exodrive/internal/core"
)

func TestDumpCurrentWindow(t *testing.T) {
	t.Setenv(exoCore.ManifestEnvVar, filepath.Join(t.TempDir(), "manifest.json"))
	useVolumesRoot(t, t.TempDir())

	item := exoCore.CacheItem{
		ID: "fake.cache", ToolName: "fake", Title: "Fake Cache",
		Category: exoCore.CategoryPackageStore, CurrentPath: "/tmp/fake-cache",
		SizeBytes: 4096, IsActive: true, RedirectMethod: exoCore.RedirectNativeConfig,
	}
	app := nsgotest.Start(t, NewMain(newFakeEngine(item)), nsgotest.Size(860, 680))
	app.WaitUntil(2*time.Second, func() bool { return app.Exists("text:Fake Cache") })
	app.Settle()
	fmt.Println("=====DUMP=====")
	fmt.Println(app.Dump())
	fmt.Println("=====END=====")
}
