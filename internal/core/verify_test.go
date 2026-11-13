package core

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"exodrive/internal/fsutil"
)

// moveCache copies a small tree to a new path and returns the record that a
// provider would write after the copy verifies. The original stays in place.
func moveCache(t *testing.T, id string) MigrationRecord {
	t.Helper()
	src := t.TempDir()
	if err := os.MkdirAll(filepath.Join(src, "blobs"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "blobs", "a.bin"), []byte("first file"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "index.json"), []byte(`{"models":[]}`), 0644); err != nil {
		t.Fatal(err)
	}

	dst := filepath.Join(t.TempDir(), "copy")
	if err := fsutil.CopyDir(src, dst, 0, nil); err != nil {
		t.Fatalf("CopyDir failed: %v", err)
	}
	summary, err := fsutil.VerifyTreeSummary(src, dst, nil)
	if err != nil {
		t.Fatalf("VerifyTreeSummary failed: %v", err)
	}

	return MigrationRecord{
		ID:             id,
		ToolName:       "npm",
		OriginalPath:   src,
		DestinationDir: dst,
		RedirectMethod: RedirectSymlink,
		MigratedAt:     time.Now(),
		SizeBytes:      summary.Bytes,
		TreeDigest:     string(summary.Digest),
		FileCount:      summary.Files,
		TotalBytes:     summary.Bytes,
	}
}

func TestVerifyAcceptsTheRecordedCopy(t *testing.T) {
	rec := moveCache(t, "npm.cache")

	result, err := NewEngine().Verify(rec)
	if err != nil {
		t.Fatalf("Verify failed: %v", err)
	}
	if result.State != VerifyOK {
		t.Fatalf("state is %q (%s), want ok", result.State, result.Detail)
	}
	if result.Files != 2 || result.Bytes != rec.TotalBytes {
		t.Fatalf("result is %+v, want the recorded counts", result)
	}
}

// TestVerifySurvivesTheLocalDelete is the point of the digest: the check works
// after the local source is gone.
func TestVerifySurvivesTheLocalDelete(t *testing.T) {
	rec := moveCache(t, "npm.cache")
	if err := os.RemoveAll(rec.OriginalPath); err != nil {
		t.Fatal(err)
	}

	result, err := NewEngine().Verify(rec)
	if err != nil {
		t.Fatalf("Verify failed: %v", err)
	}
	if result.State != VerifyOK {
		t.Fatalf("state is %q (%s), want ok", result.State, result.Detail)
	}
}

func TestVerifyReportsADamagedCopy(t *testing.T) {
	rec := moveCache(t, "npm.cache")
	if err := os.WriteFile(filepath.Join(rec.DestinationDir, "blobs", "a.bin"), []byte("first filf"), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := NewEngine().Verify(rec)
	if err != nil {
		t.Fatalf("Verify failed: %v", err)
	}
	if result.State != VerifyChanged {
		t.Fatalf("state is %q, want changed", result.State)
	}
	if result.Detail == "" {
		t.Fatal("a changed copy needs a reason")
	}
}

func TestVerifyReportsAMissingCopy(t *testing.T) {
	rec := moveCache(t, "npm.cache")
	if err := os.RemoveAll(rec.DestinationDir); err != nil {
		t.Fatal(err)
	}

	result, err := NewEngine().Verify(rec)
	if err != nil {
		t.Fatalf("Verify failed: %v", err)
	}
	if result.State != VerifyMissing {
		t.Fatalf("state is %q, want missing", result.State)
	}
}

// TestVerifyNeverCallsAnOldRecordOK keeps a record that predates the digest
// honest: the tool cannot prove a copy it never measured.
func TestVerifyNeverCallsAnOldRecordOK(t *testing.T) {
	rec := moveCache(t, "npm.cache")
	rec.TreeDigest = ""

	result, err := NewEngine().Verify(rec)
	if err != nil {
		t.Fatalf("Verify failed: %v", err)
	}
	if result.State != VerifyUnverified {
		t.Fatalf("state is %q, want unverified", result.State)
	}
}

func TestVerifyAllWalksTheLedger(t *testing.T) {
	t.Setenv(ManifestEnvVar, filepath.Join(t.TempDir(), "ledger.json"))

	good := moveCache(t, "npm.cache")
	good.ToolName = "npm"
	if err := RecordMigration(good); err != nil {
		t.Fatal(err)
	}

	damaged := moveCache(t, "cargo.registry")
	damaged.ToolName = "cargo"
	if err := RecordMigration(damaged); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(damaged.DestinationDir, "index.json"), []byte("damaged"), 0644); err != nil {
		t.Fatal(err)
	}

	results, err := NewEngine().VerifyAll()
	if err != nil {
		t.Fatalf("VerifyAll failed: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("checked %d copies, want 2", len(results))
	}
	if results[0].ID != "cargo.registry" || results[0].State != VerifyChanged {
		t.Fatalf("first result is %+v, want the changed cargo copy", results[0])
	}
	if results[1].ID != "npm.cache" || results[1].State != VerifyOK {
		t.Fatalf("second result is %+v, want the good npm copy", results[1])
	}
}

func TestVerifyAllHandlesAnEmptyLedger(t *testing.T) {
	t.Setenv(ManifestEnvVar, filepath.Join(t.TempDir(), "ledger.json"))

	results, err := NewEngine().VerifyAll()
	if err != nil {
		t.Fatalf("VerifyAll failed: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("checked %d copies, want none", len(results))
	}
}

// TestVerifyReportsAVersionOneRecordAsUnverified covers a ledger written by an
// older build, whose records hold no digest.
func TestVerifyReportsAVersionOneRecordAsUnverified(t *testing.T) {
	ledger := filepath.Join(t.TempDir(), "old.json")
	t.Setenv(ManifestEnvVar, ledger)

	copyDir := filepath.Join(t.TempDir(), "npm-cache")
	if err := os.MkdirAll(copyDir, 0755); err != nil {
		t.Fatal(err)
	}
	old := fmt.Sprintf(`{"version": 1, "records": {"npm.cache": {
		"id": "npm.cache",
		"tool_name": "npm",
		"original_path": "/home/user/.npm",
		"destination_dir": %q,
		"redirect_method": "native_config",
		"migrated_at": "2026-01-01T00:00:00Z",
		"size_bytes": 4096
	}}}`, copyDir)
	if err := os.WriteFile(ledger, []byte(old), 0644); err != nil {
		t.Fatal(err)
	}

	results, err := NewEngine().VerifyAll()
	if err != nil {
		t.Fatalf("VerifyAll failed: %v", err)
	}
	if len(results) != 1 || results[0].State != VerifyUnverified {
		t.Fatalf("results are %+v, want one unverified record", results)
	}
}
