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
packaging/Info.plist.in   # app bundle template
scripts/make-app.sh       # bundle and sign Exodrive.app
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
5. The provider calls `fsutil.VerifyTreeProgress`. It reads every regular file
   in both trees and compares the bytes by SHA-256. It also compares the file
   count, the per-file sizes, and every symbolic link with its target. A single
   different byte stops the migration before any switch. The check reports its
   progress through `MigrateProgress.Verify`. A single-file copy goes through
   `fsutil.VerifyFile`, which makes the same comparison.
   `CopyDir` refuses a destination that is not empty, is a symbolic link, or is
   a file, because a merge would overwrite data the copy does not own.
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
   - The check reads `ps -axo pid=,args=` and matches the executable and the
     arguments. A tool that starts through an interpreter is still found:
     `node .../npm-cli.js` matches `npm`. A flag token is skipped, so
     `--tools=cargo` does not match `cargo`. A shell assignment is skipped, so
     `CARGO_HOME=/tmp/cargo exodrive migrate` does not match `cargo`.
   - An unreadable process table is an error. The migration does not run blind.
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
8. **pnpm store layout**:
   - `pnpm store path` reports `<store-dir>/v<N>`. pnpm adds that folder to
     store-dir on every run.
   - Exodrive moves the store root and keeps the version folder. The new
     store-dir then holds `v<N>` again, so pnpm finds the packages.
   - The store root is the unit of the move, not the version folder.
   - A recorded move whose store path is missing stops the scan with a rollback
     hint. An empty store would send pnpm to a new location without the
     packages.
9. **Tool versions**:
   - `CacheItem.ToolVersion` holds the version the provider read, when it reads
     one. The store folder name carries it for pnpm. The window and `check`
     show it.
   - Yarn 2 and later read `cacheFolder` from each project's `.yarnrc.yml`. A
     global switch does not reach those projects, so the item carries
     `BlockedReason` and Exodrive refuses to move it.
   - `BlockedReason` keeps an item visible. `Engine.Preflight` refuses it, the
     window disables its checkbox, and every run prints the reason.
10. **Rollback record check**:
    - `loadRecord` validates the record before anything else runs.
    - Both paths must be absolute. They must differ, must not nest, and the
      destination must not be the root, the home directory, or a path with
      fewer than three segments below `/`.
11. **Content verification**:
    - `VerifyTree` reads every regular file in the source tree and in the copy.
      It compares the SHA-256 digests. A missing file, a per-file size
      difference, or a different digest is an error.
    - File count and total size alone pass a copy that damaged a byte, or that
      swapped two files of the same length. Exodrive does not rely on them.
    - `VerifyFile` checks a single copied file the same way. The Docker image
      file and the rollback staging file use it.
    - Verification reads both trees. A copy of a large cache therefore takes
      about the time of a second copy before the switch runs.
12. **Special files**:
    - A socket, a FIFO, or a device file cannot be copied or read, and a read
      would block. `CopyDir` and `VerifyTree` refuse such a tree by path.
    - The user stops the tool that owns the file, then runs the migration
      again.
13. **Tool answers for its own path**:
    - npm, pip, yarn, and uv report their cache folder through their own
      command: `npm config get cache`, `pip cache dir`, `yarn cache dir`,
      `uv cache dir`. Each reads a configuration file that can name any folder.
    - Exodrive moves the folder that the tool reports. It never moves a guessed
      folder, because the tool would keep reading the other one.
    - When no command answers, the item carries `BlockedReason` with the
      command and the step that fixes it. A device may hold `pip`, `pip3`, or
      only `python3 -m pip`; Exodrive asks each in turn.
    - bun prints its cache only inside a project, so Exodrive takes the folder
      from `BUN_INSTALL`, or `~/.bun` when the variable is unset. `bun pm cache`
      answers first when it can.
    - The tool version decides the layout. Yarn 2 and later are blocked (item
      9). Docker Desktop 4.15 and later hold `Docker.raw`; earlier versions
      hold `Docker.qcow2`. Exodrive detects both names.
14. **Target folder check**:
    - `Engine.CheckTarget` and `Engine.Preflight` refuse a folder that does not
      exist.
    - A folder under the volumes root whose volume is missing reports a
      disconnected volume, not a missing folder. The user connects the volume,
      then checks again.
    - The window names the mounted external volumes on the first run. With none
      connected, it says so, because the folder picker shows only the internal
      disk.

## 4. Failure modes

| Situation | Result |
|---|---|
| Target on exFAT, NTFS, or an unknown filesystem | Preflight error, nothing copied |
| Target lacks free space (needs size × 1.1) | Preflight error, nothing copied |
| Tool is running | Preflight error, nothing copied |
| Process table unreadable | Preflight error, nothing copied |
| Environment variable pins the cache path | Preflight error, nothing copied |
| Tool does not answer its path query | Item shows Blocked, nothing copied |
| Target folder does not exist | Preflight error, nothing copied |
| External volume is not connected | Check reports the volume, Preflight error |
| Copy fails | Error, local data intact |
| Cache holds a socket, a FIFO, or a device file | Error, local data intact |
| Verification fails | Error, local data intact, partial copy stays for inspection |
| Switch fails | Error, the ledger record is removed, the local backup is restored |
| Ledger write fails | Error, nothing switched |
| Rollback without a ledger record | Error, nothing changed |
| Rollback with the destination missing | Error, record kept |
| Rollback with a local tree that differs from the copy | Error, both copies kept, record kept |
| Second migration of a recorded item | Error until rollback runs |
| Provider detection fails | Warning with the items the other providers found |

## 5. Window contract

- The window reads signals inside `core.Dynamic`, `core.Each`, `core.Show`, and
  `...Func` fields only. A signal read at build time does not repaint.
- The window shell is built once: the header, the drive panel, the list header,
  and the footer. The drive panel holds the folder field. A field mounted
  inside a `Dynamic` remounts on every repaint and reports an empty value, which
  clears the typed path. Keep the field, and every container that holds it,
  outside any region.
- Nothing is selected after a scan. One press must not move a whole disk.
- A move needs a folder that passed `Engine.CheckTarget` (`targetState.Ready`)
  and at least one selected cache that can move. While no cache can move, the
  main action is absent. A folder that fails the check shows its reason under
  the field, and a result that a newer edit made stale is dropped.
- A folder on the volume that holds this Mac frees no space. `TargetInfo` says
  so, and the window warns under the field and again in the confirmation. The
  warning never stops a move, and the command line prints the same sentence.
- The main action names the next step: `Choose folder`, `Select all N`, or
  `Move N caches`. A move asks for confirmation with the count, the total
  bytes, the target folder, and up to five cache names.
- The header shows two gauges: the free space and cache bytes of this Mac, and
  the free space and moved bytes of the drive. Each bar part keeps a minimum
  width, so a small share stays visible. Without a chosen folder, the right
  gauge names the connected volume, or says that none is connected.
- A cache is a card, and the card keeps every fact visible: checkbox, title,
  size, state chip, folder, copy, the note that names the change Exodrive makes,
  the failure with its next step, and the one action that fits the state. No
  fact waits behind a press.
- The state chip carries a word, so color is never the only signal:
  `On this Mac`, `On the drive`, `Cannot move`, `Unused`, `Move failed`,
  `Move back failed`.
- `stateOf` reads the move first, then use, then the block. A moved cache shows
  where it lives even when it also carries a reason.
- A cache that is moved, unused, or blocked shows a disabled checkbox.
  `movable` is the only gate that select all, the move, and the count of caches
  that can move read.
- A card offers the action that fits its place. A local cache offers `Move`,
  which asks for a confirmation that holds that cache alone; without a checked
  folder it opens the folder picker first. A moved cache offers `Move back`. An
  unused cache offers `Delete cache`. Each one asks for confirmation. The
  delete names the path and says the data is gone.
- The rail lists what the check found, by group: Node, Python, Rust, Go,
  Docker, and Other. Each group shows its count and a bar split by place, so a
  press on a group reads before it filters. A group with no cache is absent.
- A place filter above the list keeps `On this Mac` or `On the drive`. The
  words match the state chip on every card.
- The rail and the place filter both narrow the list. A move acts on the caches
  on screen: `selectAll`, the count of caches that can move, and the main
  action all read the narrowed list.
- A filter change drops the selection. A press on Move never moves a cache that
  the filter hides.
- Every user-facing failure goes through `explain(err, action)` in
  `desktop/plain.go`. It returns a title, a body, and a next step. An error that
  matches no rule keeps the engine's own text in the body, so the window never
  hides a reason.
- A failure stays on the card that holds it, so the reason never waits behind a press.
  `Try again` repeats the action that stopped, and each retry asks for
  confirmation again.
- A move names its phase and its position: `Moving <title>`, `Checking the copy
  of <title>`, `Finishing <title>`, with `Cache 2 of 4. 45% done.` A stopped
  move reads `Stopping after this cache.` as soon as the press lands, so the
  press has an answer before the current cache ends.
- Escape, or a press outside a confirmation, closes it and reports that nothing
  changed. A dialog closed by selection (`base.ReasonSelect`) reports nothing.
- Progress comes from the provider's copy callback, posted to the UI thread
  through the root scope. A Dynamic branch scope ends when the branch repaints.
- A scan, a move, a move back, and a delete exclude each other. `busy` reads
  the four signals.
- Every long action rescans before it reports, so the list matches the ledger.
- The footer shows the status line and a `Details` toggle that names the record
  file a move back needs.
- The folder picker lists nothing until it is told a folder. The window calls
  `filepicker.State.Open` as the picker opens, at the chosen folder or at the
  home folder, so the dialog never opens empty.
- The window reads the login shell PATH before the first scan. A window opened
  from Finder or the Dock inherits the launchd PATH, which holds no Homebrew,
  nvm, or custom folder. Without that step, every tool query fails and each
  cache shows `Cannot move`.

## 6. Command line contract

- `check` prints the detected caches, the tool version, and the switch method.
  A migrated cache prints its copy as `   └── copy: <path>`.
  A provider that fails prints a warning; the other items stay usable. An
  unreadable ledger stops the command.
- `preflight <item> --target` runs the same checks a migration runs first.
- `migrate --target` moves the selected caches. The command prints a blocked
  cache as `[blocked] <title>: <reason>` and leaves it out of the run. When
  every selected cache is blocked, it exits 1. `--dry-run` runs every check
  and prints the plan. It copies no data, switches no tool, and writes no
  record. A blocked item makes the command exit 1.
- A move prints the copy progress bar, then `Checking the copy...` with a
  second bar. Both bars read the same source size.
- `rollback <item>` restores one cache and removes its record.
- `orphans [--clean]` lists or deletes abandoned caches.
- `version` prints the build version. A release build shows its number with a
  `v` prefix; a development build shows its commit or `dev`.

## 7. Release contract

- `release.yml` runs the test suite, builds four CLI targets, builds two app
  bundles on macOS, writes `SHA256SUMS`, and publishes the release.
- The CI bundle is ad-hoc signed, because the workflow holds no signing
  identity. `make app` with `SIGN_IDENTITY` produces a Developer ID build.
- A Developer ID build carries `--options runtime` and `--timestamp`.
  Notarization refuses a bundle without both, so the two paths cannot mix.
- `make notarize` needs `NOTARY_PROFILE`. `scripts/notarize.sh` stops when the
  profile is missing, when the signature is not a Developer ID, when Apple
  rejects the upload, or when Gatekeeper still rejects the stapled bundle.
- A bundle that carries no ticket keeps the Gatekeeper prompt. It must not ship
  as a notarized build.
