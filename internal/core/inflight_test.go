package core

import (
	"os"
	"path/filepath"
	"testing"
)

// The record survives a start that finds it, and a finished action leaves none.
func TestInFlightSurvivesAStop(t *testing.T) {
	t.Setenv(ManifestEnvVar, filepath.Join(t.TempDir(), "manifest.json"))

	if _, ok := ReadInFlight(); ok {
		t.Fatal("a fresh folder holds an action")
	}
	if err := WriteInFlight(InFlight{ID: "npm.cache", Title: "NPM Cache", Action: "move", Target: "/Volumes/SSD/exodrive"}); err != nil {
		t.Fatal(err)
	}
	got, ok := ReadInFlight()
	if !ok || got.ID != "npm.cache" || got.Title != "NPM Cache" || got.StartedAt.IsZero() {
		t.Fatalf("the record reads %+v", got)
	}
	if err := ClearInFlight(); err != nil {
		t.Fatal(err)
	}
	if _, ok := ReadInFlight(); ok {
		t.Fatal("the record survived its end")
	}
	// A second end finds nothing and is not an error.
	if err := ClearInFlight(); err != nil {
		t.Fatalf("a clear without a record failed: %v", err)
	}
}

// A file the window cannot read tells it nothing, and never stops the start.
func TestInFlightIgnoresARecordItCannotRead(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(ManifestEnvVar, filepath.Join(dir, "manifest.json"))
	if err := os.WriteFile(InFlightPath(), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, ok := ReadInFlight(); ok {
		t.Fatal("an unreadable record was read")
	}
	if err := os.WriteFile(InFlightPath(), []byte(`{"title":"no id"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, ok := ReadInFlight(); ok {
		t.Fatal("a record without an id was read")
	}
}
