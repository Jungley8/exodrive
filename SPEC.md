# SPEC.md — Exodrive Specification

## 1. Overview

Exodrive moves developer tool caches from the internal disk to an external
volume. It provides a command line tool and an nsgo desktop window.

A migration is a move. Exodrive copies the cache, verifies the copy, switches
the tool to the copy, records the move, and then deletes the local copy. A
rollback moves the cache back.

Supported toolchains:
- Node ecosystem: `npm`, `pnpm`, `yarn`, `bun`
- Rust: `cargo` (registry, git)
- Go: `go-build` (GOCACHE), `gomod` (GOMODCACHE)
- Python: `uv` (cache and Python toolchains), `pip`
- Containers: `docker` (OrbStack, Docker Desktop, Colima)
- Legacy/Orphans: abandoned cache folders from earlier configuration changes

## 2. Architecture & Contracts

```
cmd/
  exodrive/               # CLI
  exodrive-gui/           # nsgo desktop window
internal/
  core/                   # Engine, scan, preflight, ledger, orphan cleaning
  fsutil/                 # copy, verify, symlink, disk check, process lock
  plugins/                # Providers for each toolchain
    provider.go           # Provider interface and registry
    rollback.go           # shared rollback and cleanup helpers
    npm.go pnpm.go yarn.go cargo.go golang.go bun.go uv.go pip.go docker.go orphan.go
  ui/                     # terminal table and progress bar
desktop/                  # nsgo reactive window
.github/workflows/        # CI and release
```

### Provider contract

```go
type Provider interface {
    Name() string
    Detect(ctx context.Context) ([]core.CacheItem, error)
    PreflightCheck(ctx context.Context, item core.CacheItem, targetRoot string) error
    Migrate(ctx context.Context, item core.CacheItem, targetRoot string, onProgress fsutil.CopyProgressCallback) error
    Rollback(ctx context.Context, item core.CacheItem) error
}
```

### Migration sequence

1. `Engine.Migrate` looks up the provider. A missing provider is an error.
2. `Engine.Migrate` refuses an item that the ledger already holds.
3. `Engine.Preflight` checks the target filesystem, the free space, the
   provider's process lock, and the environment overrides.
4. The provider copies the data with `fsutil.CopyDir` or `fsutil.CopyFile`.
   Both preserve the file mode and the modification time. `CopyDir` restores
   directory modes and times after the copy.
5. The provider calls `fsutil.VerifyTree`. It compares the regular-file count
   and the total size of both trees. A mismatch stops the migration before any
   switch.
6. The provider calls `core.RecordMigration`. The record exists before the
   switch, so a process that dies after the switch still has a record. The
   ledger refuses a second record for the same item ID.
7. The provider switches the tool:
   - Native configuration: `npm config set`, `pnpm config set`, `yarn config
     set ... --global`, `pip config set`, `go env -w`, or the `data_dir` key in
     `~/.orbstack/config.json` written atomically.
   - Symbolic link: `fsutil.ReplaceDirWithSymlink` moves the local entry to
     `path.bak`, creates a temporary link, and renames it into place.
   - A failed switch removes the record, so the user can retry.
8. The provider removes the local copy to reclaim the space.

A failure before step 6 leaves the local data intact. A failure at step 8 leaves
a ledger record and duplicated data. A rollback recovers both cases.

### Rollback sequence

1. The provider loads the ledger record. A missing record is an error.
2. Native configuration: the provider restores the previous configuration
   value. A configuration command failure is returned, never ignored.
3. The provider copies the data back from the destination to a staging path
   beside the original, verifies the copy, then renames it into place. When
   the local path holds a symlink, the link is removed first. If the copy back
   fails, the link is recreated so the tool keeps working.
4. The provider removes the destination copy.
5. The provider removes the ledger record.

Rollback reconciles an interrupted run. When the local path already holds data,
Exodrive compares it with the destination. Identical trees come from an earlier
rollback, so Exodrive removes the destination. Different trees keep both copies
and return an error with the record intact.

## 3. Invariants & Fail-Closed Guards

1. **Filesystem allowlist**:
   - The target must report `apfs`, `hfs`, `hfs+`, `ext4`, `ext3`, `ext2`,
     `btrfs`, `zfs`, `xfs`, `f2fs`, or `bcachefs`.
   - macOS reads the name from `Statfs_t.Fstypename`. Linux maps the
     `Statfs_t.Type` magic number, and reports any other value as a hex number.
     A platform without a mapping reports an unknown name.
   - An unknown filesystem is refused. The list is an allowlist, not a denylist.
2. **Process lock**:
   - `docker` refuses to run while Docker Desktop, OrbStack, or Colima runs.
   - Each package manager refuses to run while its own process runs.
   - The check reads process names with `pgrep`. It is a guard, not a proof.
3. **Environment overrides**:
   - npm, pnpm, yarn, Go, and pip accept a cache path from the environment. A
     set variable wins over the tool's own configuration, so the switch would
     not take effect and the local copy would be deleted anyway.
   - Preflight refuses the migration and names the variable. The user unsets it
     and runs the migration again.
4. **Binary isolation**:
   - Exodrive never moves `~/.cargo/bin`, `~/.bun/bin`, or `$GOPATH/bin`.
   - Cargo moves only `registry/` and `git/`. Bun moves only `install/cache`.
5. **Cross-volume hardlink notice**:
   - The pnpm store and the uv cache use hard links or clones. A store on
     another volume makes pnpm and uv copy files. The item carries a warning.
6. **Ledger and rollback**:
   - The ledger is `~/.exodrive/manifest.json`, overridden by
     `EXODRIVE_MANIFEST_PATH`.
   - Writes are atomic and durable: a unique temporary file, a flush, a rename,
     then a flush of the directory.
   - Writers take an exclusive lock on `manifest.json.lock`, so two Exodrive
     processes cannot lose an update.
   - A record ID is unique. A second record for the same ID is refused.
   - A scan marks an item as migrated when the ledger holds its ID.
   - A rollback removes the record only after it restores the data. A rollback
     that cannot restore the data keeps the record.
7. **Orphan deletion guard**:
   - `CleanOrphan` accepts an absolute path only.
   - It refuses `/`, the user home directory, a top-level folder inside the home
     directory, and any path with fewer than three segments below `/`.

## 4. Failure modes

| Situation | Result |
|---|---|
| Target on exFAT, NTFS, or an unknown filesystem | Preflight error, nothing copied |
| Target lacks free space (needs size × 1.1) | Preflight error, nothing copied |
| Tool is running | Preflight error, nothing copied |
| Environment variable pins the cache path | Preflight error, nothing copied |
| Copy fails | Error, local data intact |
| Verification fails | Error, local data intact, partial copy stays for inspection |
| Switch fails | Error, the ledger record is removed, the local backup is restored |
| Ledger write fails | Error, nothing switched |
| Rollback without a ledger record | Error, nothing changed |
| Rollback with the destination missing | Error, record kept |
| Rollback with a local tree that differs from the copy | Error, both copies kept, record kept |
| Second migration of a recorded item | Error until rollback runs |
| Provider detection fails | Warning with the items the other providers found |

## 5. Window contract

- The window reads signals inside `core.Dynamic`, `core.Each`, and `...Func`
  fields only. A signal read at build time does not repaint.
- A migration requires a non-empty target folder. The Migrate button stays
  disabled until the field holds one.
- A migration asks for confirmation and shows the number of caches and the
  target folder.
- Progress comes from the provider's copy callback, posted to the UI thread
  through the root scope. A Dynamic branch scope ends when the branch repaints.
- A rescan and a migration cannot run at the same time.
- The window does not run a rollback. The command line does.
