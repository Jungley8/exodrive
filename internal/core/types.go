package core

import "time"

// CacheCategory classifies the role of the cache item.
type CacheCategory string

const (
	CategoryPackageStore CacheCategory = "package_store" // Package download store (npm, pnpm, cargo registry, gomod, uv, pip)
	CategoryBuildCache   CacheCategory = "build_cache"   // Build output cache (go-build, cargo git)
	CategoryModelStore   CacheCategory = "model_store"   // AI model weights (ollama, Hugging Face, LM Studio)
	CategoryVirtualDisk  CacheCategory = "virtual_disk"  // VM virtual disk images (Docker, OrbStack, Colima)
	CategoryOrphan       CacheCategory = "orphan"        // Historical abandoned caches
)

// RedirectMethod describes how the tool redirects access to the target path.
type RedirectMethod string

const (
	RedirectNativeConfig RedirectMethod = "native_config" // Official CLI config (go env, npm config, etc.)
	RedirectSymlink      RedirectMethod = "symlink"       // Precise subdirectory symbolic link
	RedirectManual       RedirectMethod = "manual"        // Guided manual configuration
)

// MovePhase names the step of a migration that runs now.
type MovePhase string

const (
	// PhaseCopy copies the cache to the target volume.
	PhaseCopy MovePhase = "copy"
	// PhaseVerify reads both trees and compares the content.
	PhaseVerify MovePhase = "verify"
)

// LinkState says how the original path stands for a record that moved by
// symbolic link. A scan reads the path, because the ledger alone cannot say
// whether a tool wrote the folder again after the move.
type LinkState string

const (
	// LinkUnknown means the scan cannot tell: the ledger holds no record for
	// the item, the record moved the data with a tool setting, or the original
	// path is gone.
	LinkUnknown LinkState = ""
	// LinkMoved means the original path is a link to the copy on the drive.
	LinkMoved LinkState = "moved"
	// LinkRewritten means the tool no longer reaches the copy through the
	// original path, because a real folder sits there again or the link points
	// somewhere else.
	LinkRewritten LinkState = "rewritten"
)

// CacheItem represents a single developer cache entity.
type CacheItem struct {
	ID          string        `json:"id"`
	ToolName    string        `json:"tool_name"`
	ToolVersion string        `json:"tool_version,omitempty"`
	Title       string        `json:"title"`
	Category    CacheCategory `json:"category"`
	CurrentPath string        `json:"current_path"`
	// DestinationPath holds the moved copy from the ledger record, or "".
	DestinationPath string `json:"destination_path,omitempty"`
	ConfiguredPath  string `json:"configured_path,omitempty"`
	SizeBytes       int64  `json:"size_bytes"`
	IsActive        bool   `json:"is_active"`
	IsMigrated      bool   `json:"is_migrated"`
	IsSymlink       bool   `json:"is_symlink"`
	// LinkState says where the move stands for a record that used a link. The
	// scan fills it. Empty for a cache with no record.
	LinkState      LinkState      `json:"link_state,omitempty"`
	RedirectMethod RedirectMethod `json:"redirect_method"`
	TargetSubdir   string         `json:"target_subdir"`
	Warning        string         `json:"warning,omitempty"`
	// BlockedReason says why Exodrive cannot move this item. Empty when it can.
	// A blocked item stays visible, so the user can read the reason and act.
	BlockedReason string `json:"blocked_reason,omitempty"`
}

// MigrationRecord is saved in ~/.exodrive/manifest.json.
type MigrationRecord struct {
	ID             string         `json:"id"`
	ToolName       string         `json:"tool_name"`
	OriginalPath   string         `json:"original_path"`
	DestinationDir string         `json:"destination_dir"`
	RedirectMethod RedirectMethod `json:"redirect_method"`
	OriginalConfig string         `json:"original_config,omitempty"`
	MigratedAt     time.Time      `json:"migrated_at"`
	SizeBytes      int64          `json:"size_bytes"`
	// TreeDigest identifies the content of the copy at the time of the move,
	// so `exodrive verify` can prove the copy later. A record written before
	// version 2 holds no digest, and a check reports it as unverified.
	TreeDigest string `json:"tree_digest,omitempty"`
	FileCount  int64  `json:"file_count,omitempty"`
	TotalBytes int64  `json:"total_bytes,omitempty"`
}

// Manifest tracks all migrations made by exodrive for safe rollback.
type Manifest struct {
	Version   int                        `json:"version"`
	UpdatedAt time.Time                  `json:"updated_at"`
	Records   map[string]MigrationRecord `json:"records"`
}

// ManifestVersion is the version of the ledger that this build writes.
// Version 2 adds the content digest of each copy.
const ManifestVersion = 2
