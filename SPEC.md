# SPEC.md — Exodrive Specification

## 1. Overview
Exodrive is a Go-based utility (CLI and nsgo native desktop app) designed to inspect, safely migrate, and rollback developer tool caches across filesystem boundaries (such as to external NVMe/APFS drives).

Supported toolchains:
- Node ecosystem: `npm`, `pnpm`, `yarn`, `bun`
- Rust: `cargo` (registry, git, cache)
- Go: `go-build` (GOCACHE), `gomod` (GOMODCACHE)
- Python: `uv` (cache and python toolchains), `pip`
- Containers: `docker` (OrbStack, Docker Desktop, Colima)
- Legacy/Orphans: historical abandoned cache folders

## 2. Architecture & Contracts

```
cmd/
  exodrive/               # CLI and terminal interface
  exodrive-gui/           # nsgo native desktop GUI
internal/
  core/                   # Engine, Scanner, Migrator, Rollback, Manifest
  fsutil/                 # APFS checking, safe copy, process locking, symlink
  plugins/                # Providers for each toolchain
    provider.go           # Provider interface
    npm.go
    pnpm.go
    yarn.go
    cargo.go
    golang.go
    bun.go
    uv.go
    pip.go
    docker.go
    orphan.go
  manifest/               # ~/.exodrive/manifest.json ledger
desktop/                  # nsgo reactive desktop UI components
```

### Provider Contract
```go
type Provider interface {
    Name() string
    Detect(ctx context.Context) ([]core.CacheItem, error)
    PreflightCheck(ctx context.Context, item core.CacheItem, targetRoot string) error
    Migrate(ctx context.Context, item core.CacheItem, targetRoot string, onProgress func(copied, total int64)) error
    Rollback(ctx context.Context, item core.CacheItem) error
}
```

## 3. Invariants & Fail-Closed Guards

1. **Filesystem Pre-flight**:
   - Destination volume must be `APFS` or `HFS+` on macOS (or `ext4`/`btrfs`/`zfs` on Linux).
   - If destination filesystem is `exFAT`, `FAT32`, or `NTFS`, migration fails immediately with an explicit error.
2. **Process Locking Guard**:
   - For `docker`: if Docker Desktop or OrbStack is actively running, refuse migration until the engine is stopped.
   - For build caches: warn or block if active compile/package manager processes (`cargo`, `go`, `pnpm`) are running.
3. **Binaries Preservation Guard**:
   - Never migrate `bin/` directories (e.g. `~/.cargo/bin`, `~/.bun/bin`, `$GOPATH/bin`).
   - Only data caches are migrated (`registry`, `git`, `pkg/mod`, `cache`).
4. **Cross-Volume Hardlink Notice**:
   - For `pnpm` and `uv`, if project workspace is on a different filesystem than the target store, mark with a warning indicating fallback to copy mode.
5. **Ledger & Atomic Rollback**:
   - Every migration is recorded in `~/.exodrive/manifest.json`.
   - Rollback must restore native configs or delete symlinks and restore local directories atomically.
