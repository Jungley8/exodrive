package core

import "time"

// CacheCategory classifies the role of the cache item.
type CacheCategory string

const (
	CategoryPackageStore CacheCategory = "package_store" // Package download store (npm, pnpm, cargo registry, gomod, uv, pip)
	CategoryBuildCache   CacheCategory = "build_cache"   // Build output cache (go-build, cargo git)
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

// CacheItem represents a single developer cache entity.
type CacheItem struct {
	ID             string         `json:"id"`
	ToolName       string         `json:"tool_name"`
	ToolVersion    string         `json:"tool_version,omitempty"`
	Title          string         `json:"title"`
	Category       CacheCategory  `json:"category"`
	CurrentPath    string         `json:"current_path"`
	ConfiguredPath string         `json:"configured_path,omitempty"`
	SizeBytes      int64          `json:"size_bytes"`
	IsActive       bool           `json:"is_active"`
	IsMigrated     bool           `json:"is_migrated"`
	IsSymlink      bool           `json:"is_symlink"`
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
}

// Manifest tracks all migrations made by exodrive for safe rollback.
type Manifest struct {
	Version   int                        `json:"version"`
	UpdatedAt time.Time                  `json:"updated_at"`
	Records   map[string]MigrationRecord `json:"records"`
}
