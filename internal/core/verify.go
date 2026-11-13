package core

import (
	"fmt"
	"os"
	"sort"

	"exodrive/internal/fsutil"
)

// VerifyState is the answer to a check of a recorded copy.
type VerifyState string

const (
	// VerifyOK means the copy holds the bytes that the move recorded.
	VerifyOK VerifyState = "ok"
	// VerifyChanged means the copy differs from the recorded digest.
	VerifyChanged VerifyState = "changed"
	// VerifyMissing means the copy is not at the recorded path.
	VerifyMissing VerifyState = "missing"
	// VerifyUnverified means the record holds no digest, so no check is possible.
	VerifyUnverified VerifyState = "unverified"
)

// VerifyResult reports one check.
type VerifyResult struct {
	ID     string      `json:"id"`
	Tool   string      `json:"tool_name"`
	Copy   string      `json:"destination_dir"`
	State  VerifyState `json:"state"`
	Detail string      `json:"detail,omitempty"`
	Files  int64       `json:"files,omitempty"`
	Bytes  int64       `json:"bytes,omitempty"`
}

// Verify reads a recorded copy and compares its content with the digest that
// the move recorded. It never writes to the drive and never changes the ledger.
func (e *Engine) Verify(rec MigrationRecord) (VerifyResult, error) {
	result := VerifyResult{
		ID:    rec.ID,
		Tool:  rec.ToolName,
		Copy:  rec.DestinationDir,
		Files: rec.FileCount,
		Bytes: rec.TotalBytes,
	}

	if rec.TreeDigest == "" {
		result.State = VerifyUnverified
		result.Detail = "The ledger holds no digest for this move. Move the cache back, then move it again."
		return result, nil
	}

	summary, err := fsutil.DigestPath(rec.DestinationDir)
	if err != nil {
		if os.IsNotExist(err) {
			result.State = VerifyMissing
			result.Detail = "The copy is not at the recorded path. Connect the drive, then check again."
			return result, nil
		}
		return result, fmt.Errorf("check %s: %w", rec.DestinationDir, err)
	}

	result.Files, result.Bytes = summary.Files, summary.Bytes
	if string(summary.Digest) == rec.TreeDigest {
		result.State = VerifyOK
		return result, nil
	}

	result.State = VerifyChanged
	switch {
	case summary.Bytes != rec.TotalBytes:
		result.Detail = fmt.Sprintf("The copy holds %d bytes. The move recorded %d.", summary.Bytes, rec.TotalBytes)
	case summary.Files != rec.FileCount:
		result.Detail = fmt.Sprintf("The copy holds %d files. The move recorded %d.", summary.Files, rec.FileCount)
	default:
		result.Detail = "A file in the copy differs from the move. Move the cache back, then move it again."
	}
	return result, nil
}

// VerifyID checks the copy that the ledger records for id.
func (e *Engine) VerifyID(id string) (VerifyResult, error) {
	manifest, err := LoadManifest()
	if err != nil {
		return VerifyResult{}, err
	}
	rec, ok := manifest.Records[id]
	if !ok {
		return VerifyResult{}, fmt.Errorf("no record of a move for %s", id)
	}
	return e.Verify(rec)
}

// VerifyAll checks every record in the ledger, in ID order.
func (e *Engine) VerifyAll() ([]VerifyResult, error) {
	manifest, err := LoadManifest()
	if err != nil {
		return nil, err
	}

	ids := make([]string, 0, len(manifest.Records))
	for id := range manifest.Records {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	results := make([]VerifyResult, 0, len(ids))
	for _, id := range ids {
		result, err := e.Verify(manifest.Records[id])
		if err != nil {
			return results, err
		}
		results = append(results, result)
	}
	return results, nil
}
