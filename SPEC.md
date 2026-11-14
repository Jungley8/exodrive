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
- JVM: `maven` (`repository/`), `gradle` (`caches/`, `wrapper/dists/`)
- Xcode: `xcode` (DerivedData, Archives, DeviceSupport, Previews, app cache;
  the simulator folder stays blocked)
- Apple dependency stores: `cocoapods` (cache, spec repos), `swiftpm` (cache)
- Mobile: `dart` (`hosted/`, `git/` under the pub cache), `android`
  (`build-cache/`)
- conda: `conda` (every `pkgs_dirs` entry that `conda info --json` reports)
- Homebrew: `homebrew` (`downloads/` under the Homebrew cache)
- Ruby: `ruby` (the Bundler cache, and the cache of every user gem folder)
- PHP: `composer` (the download cache)
- .NET: `nuget` (the global package folder)
- Dev tools: `devtools` (Electron downloads, node-gyp headers, the tsc cache)
- Browser test stores: `playwright` (`PLAYWRIGHT_BROWSERS_PATH`), `cypress`
  (`CYPRESS_CACHE_FOLDER`), `puppeteer` (`PUPPETEER_CACHE_DIR`)
- Go tool caches: `golangci-lint` (`GOLANGCI_LINT_CACHE`), `gopls` (the system
  cache folder)
- AI model stores: `ollama`, `huggingface` (`hub/` under `HF_HOME`), `lmstudio`
- Legacy/Orphans: abandoned cache folders from earlier configuration changes

### Platform promise

The promise is deliberately narrow. Only macOS carries a support promise.

| Platform | Window | Command line | Tests |
|---|---|---|---|
| macOS 13+ | Ships as an app bundle | Ships | Every test runs on `macos-latest` |
| Linux | Not shipped | Ships | CI runs the suite on `ubuntu-latest` |
| Windows 10+ | Ships as a zip archive | Ships | CI builds it; no test runs on Windows |

- A bug on a platform without a promise is still a bug. A fix follows the
  same checks as a fix on macOS.
- The window uses nsgo, which chooses its native toolkit at build time. nsgo
  pins a cgo-free Win32 backend with a folder picker, so the window compiles
  for `windows/amd64` and `windows/arm64`.
- Windows ships unverified. No one has run the binary on Windows hardware,
  and this specification claims nothing about how it behaves there.

## 2. Architecture & Contracts

```
cmd/
  exodrive/               # CLI
  exodrive-gui/           # nsgo desktop window
internal/
  core/                   # Engine, scan, preflight, ledger, orphan cleaning
    record.go             # the ledger record check that every delete runs first
  fsutil/                 # copy, verify, symlink, disk check, process lock
  plugins/                # Providers for each toolchain
    provider.go           # Provider interface and registry
    move.go               # shared copy, verify, record, and link step
    linked.go             # table-driven provider for fixed folders
    rollback.go           # shared rollback and cleanup helpers
    npm.go pnpm.go yarn.go cargo.go golang.go bun.go uv.go pip.go docker.go
    xcode.go jvm.go apple.go dart.go android.go conda.go homebrew.go
    ruby.go composer.go nuget.go devtools.go browsers.go gotools.go
    orphan.go
  ui/                     # terminal table and progress bar
desktop/                  # nsgo reactive window
packaging/Info.plist.in   # app bundle template
scripts/make-app.sh       # bundle and sign Exodrive.app
scripts/check-site.sh     # guard for the pages under site/
scripts/install.sh        # one-line install for macOS and Linux
scripts/install.ps1       # one-line install for Windows
site/                     # landing page and documentation, served by GitHub Pages
.github/workflows/        # CI, release, and pages
```

### Provider contract

```go
type Provider interface {
    Name() string
    Detect(ctx context.Context) ([]core.CacheItem, error)
    PreflightCheck(ctx context.Context, item core.CacheItem, targetRoot string) error
    Migrate(ctx context.Context, item core.CacheItem, targetRoot string, progress fsutil.MigrateProgress) error
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
5. The provider calls `fsutil.VerifyTreeSummary`. It reads every regular file
   in both trees and compares the bytes by SHA-256. It also compares the file
   count, the per-file sizes, and every symbolic link with its target. A single
   different byte stops the migration before any switch. The check reports its
   progress through `MigrateProgress.Verify` and returns the digest of the copy.
   A single-file copy goes through `fsutil.VerifyFileSummary`, which makes the
   same comparison and also returns a digest.
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

### Keep-local sequence

A tool that deletes the link and writes its cache again leaves an item that a
rollback cannot finish. `rollback --keep-local` takes the local folder as the
truth. The window offers it as **Keep the local folder** on the card, with no
failed press first.

1. Load the ledger record. A missing record is an error.
2. Run the record check. The check refuses a record whose paths could delete the
   wrong data.
3. The original path must hold a real folder or file. A symlink means the copy
   is still in use, so the command names the rollback instead. A missing
   original means the rollback can still restore it, so the command refuses.
4. The parent of the destination must exist. This tells a detached drive from a
   copy that someone deleted. While the drive is away, the record stays.
5. Remove the destination copy.
6. Remove the ledger record.

The local folder is never read for a digest, because the person chose it over
the copy. Nothing else in the run deletes a local path.

### Platform notes

Each system provides the same four primitives. A system that cannot provide one
refuses the migration; none of them falls back to a weaker mode.

| Primitive | macOS and Linux | Windows |
|---|---|---|
| Disk usage | `Statfs` | `GetDiskFreeSpaceExW` |
| Filesystem name | `Statfs_t.Fstypename`, or the `Type` magic number | `GetVolumeInformationW` |
| Device identity | `Stat_t.Dev` | the volume serial number |
| Mount points | `/Volumes/<name>` | ready drive roots from `GetLogicalDrives` |
| Process table | `ps -axo pid=,args=` | `Get-CimInstance Win32_Process` through PowerShell |
| Link | symbolic link | directory junction (`mklink /J`) |
| Ledger lock | `flock(LOCK_EX)` | `LockFileEx` |
| Directory flush | `Sync` | nothing to flush |

- A junction needs no administrator rights, where a symbolic link needs
  Developer Mode. A junction holds directories on NTFS only, so NTFS is the one
  Windows entry in the allowlist and exFAT stays refused.
- A file link has no Windows implementation. A provider that must link a single
  file refuses the migration there and says why.
- An unreadable process table, or a missing PowerShell, fails the process check
  closed. The migration does not run without the check.

## 3. Invariants & Fail-Closed Guards

1. **Filesystem allowlist**:
   - The target must report `apfs`, `hfs`, `hfs+`, `ext4`, `ext3`, `ext2`,
     `btrfs`, `zfs`, `xfs`, `f2fs`, `bcachefs`, or `ntfs`.
   - macOS reads the name from `Statfs_t.Fstypename`. Linux maps the
     `Statfs_t.Type` magic number, and reports any other value as a hex number.
     Windows reads the name from `GetVolumeInformationW`.
   - An unknown filesystem is refused. The list is an allowlist, not a denylist.
   - exFAT and FAT32 are refused on every system: they hold neither junctions
     nor POSIX modes.
2. **Process lock**:
   - `docker` refuses to run while Docker Desktop, OrbStack, or Colima runs.
   - `xcode` refuses to run while Xcode, `xcodebuild`, or `XCBuildService`
     runs. `CoreSimulatorService` is not in the list: it always runs, so every
     preflight would fail.
   - `maven` refuses while `mvn` or `mvnd` runs. `gradle` refuses while `gradle`
     or the daemon runs. `cocoapods` refuses while `pod` runs. `swiftpm` refuses
     while `swift`, `swiftc`, or `xcodebuild` runs.
   - `dart` refuses while `dart`, `flutter`, or `pub` runs. `android` refuses
     while `gradle` or the daemon runs. `conda` refuses while `conda`, `mamba`,
     or `micromamba` runs. `homebrew` refuses while `brew` runs.
   - `ruby` refuses while `bundle` or `gem` runs. It does not name `ruby`:
     a Rails server is not a package command. `composer` refuses while
     `composer` runs. `nuget` refuses while `dotnet`, `msbuild`, or `nuget`
     runs. `devtools` names no process: `node` runs long-lived programs, so a
     lock on it would stop a move on a machine that builds nothing.
   - `playwright`, `cypress`, and `puppeteer` name no process. The run that
     reads the store is `node`, and the browser program names are shared with
     programs that never touch the folder. A name lock would miss the writer or
     stop an unrelated program, so the row carries the next step instead.
   - `golangci-lint` refuses while `golangci-lint` runs. `gopls` names no
     process: an editor keeps it running, so the lock would stop a move on every
     machine with an open editor.
   - Each package manager refuses to run while its own process runs.
   - The check reads the process table and matches the program and the paths
     that name a tool. The first token that is neither a flag nor a shell
     assignment is the program. A later token counts only when it is a path, a
     Java main class, or a module after `-m`:
     - `node .../npm-cli.js` matches `npm`, and `python3 -m pip` matches `pip`.
     - `grep -i dart log.txt` matches nothing, because a bare word is an
       argument. The dart process itself carries the name.
     - A flag token is skipped, so `--tools=cargo` does not match `cargo`. A
       shell assignment is skipped, so `CARGO_HOME=/tmp/cargo exodrive migrate`
       does not match `cargo`.
     - A process above Exodrive is skipped. A shell or a make target that
       launched Exodrive can hold a tool path in its own command line, and it
       is not the tool.
   - A Java main class names its tool through the last segment of a dotted
     token: `org.gradle.launcher.daemon.bootstrap.GradleDaemon` matches
     `GradleDaemon`. The rule needs a capital letter, so `notes.go` does not
     match `go`.
   - macOS and Linux read `ps -axo pid=,args=`. Windows reads
     `Get-CimInstance Win32_Process` through PowerShell, because the command
     line decides whether `node.exe` runs npm.
   - An unreadable process table is an error. The migration does not run blind.
3. **Environment overrides**:
   - npm, pnpm, yarn, Go, and pip accept a cache path from the environment. A
     set variable wins over the tool's own configuration, so the switch would
     not take effect and the local copy would be deleted anyway.
   - Preflight refuses the migration and names the variable. The user unsets it
     and runs the migration again.
   - `BUNDLE_USER_CACHE`, `COMPOSER_CACHE_DIR`, and `NUGET_PACKAGES` name a
     folder that a symbolic link replaces, so Exodrive does not refuse them.
   - A model store reads its folder from `OLLAMA_MODELS` or `HF_HOME`, and
     Exodrive does not refuse it. The switch replaces the folder with a link,
     so the variable still names a path that works. The guard exists for a
     native configuration switch, where the variable would win.
   - `GRADLE_USER_HOME` and `CP_HOME_DIR` name a root folder, and Exodrive moves
     a folder inside that root. The link keeps the variable valid, so the guard
     does not apply. `PUB_CACHE` and `HOMEBREW_CACHE` follow the same rule.
   - `conda` never guesses a prefix. It reads `pkgs_dirs` from
     `conda info --json`. A missing tool adds no item, and an unreadable reply
     is an error that the scan shows.
4. **Binary isolation**:
   - Exodrive never moves `~/.cargo/bin`, `~/.bun/bin`, or `$GOPATH/bin`.
   - Cargo moves only `registry/` and `git/`. Bun moves only `install/cache`.
   - Exodrive never moves `~/Library/Developer/CoreSimulator/Devices`.
     CoreSimulatorService cannot write its device store to an external volume,
     so a link there stops device creation and breaks the Files app inside the
     simulator. The item stays visible with `BlockedReason`.
   - Exodrive never moves `~/Library/Developer/Xcode/UserData`, and it never
     moves `UserData/IB Support`. Asset catalog builds write there. Only
     `UserData/Previews` moves.
   - Gradle moves only `caches/` and `wrapper/dists/`. `daemon/`, `native/`, and
     `init.d/` stay. Maven moves only `repository/`.
   - CocoaPods moves only `repos/` out of the CocoaPods home folder, which
     `CP_HOME_DIR` may name. `trunk/` stays.
   - Dart moves only `hosted/` and `git/` out of the pub cache. `bin/` and
     `global_packages/` stay, so a global command on `PATH` keeps working when
     the drive is away. Homebrew moves only `downloads/`; `api/` stays.
   - Android moves only `~/.android/build-cache`. The emulator data stays,
     because an emulator on another volume runs far slower.
5. **Cross-volume hardlink notice**:
   - The pnpm store and the uv cache use hard links or clones. A store on
     another volume makes pnpm and uv copy files. The item carries a warning.
   - conda hardlinks packages into an environment on the same volume. Across
     volumes it copies them, so an environment uses more space. The item
     carries a warning.
6. **Ledger and rollback**:
   - The ledger is `~/.exodrive/manifest.json`, overridden by
     `EXODRIVE_MANIFEST_PATH`.
   - Writes are atomic and durable: a unique temporary file, a flush, a rename,
     then a flush of the directory on macOS and Linux. Windows cannot flush a
     directory handle, so it flushes the file only.
   - Writers take an exclusive lock on `manifest.json.lock`, so two Exodrive
     processes cannot lose an update.
   - A record ID is unique. A second record for the same ID is refused.
   - A scan marks an item as migrated when the ledger holds its ID and the
     record names the folder that the item now reports.
   - A record that names another folder stops the row: the item keeps no move
     claim, and `BlockedReason` names the recorded folder, the folder that the
     tool reads, and the rollback command. A provider can change the folder it
     reports, and the record then describes a folder that no tool reads.
   - For a record that moved the data by link, the scan reads the original path
     and sets `LinkState`. `moved` means the link reaches the copy. `rewritten`
     means a real folder sits there again, or the link points elsewhere. Empty
     means the path is gone, or the record moved the data with a tool setting.
     A path that the scan cannot read counts as `rewritten`, so the card asks
     the person to look.
   - `rewritten` keeps the record. The card offers **Keep the local folder** and
     no move back or check, because the two trees differ.
   - A rollback removes the record only after it restores the data. A rollback
     that cannot restore the data keeps the record.
7. **Orphan deletion guard**:
   - `CleanOrphan` accepts an absolute path only.
   - It refuses `/`, the user home directory, a top-level folder inside the home
     directory, and any path with fewer than three segments below the volume
     root.
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
    - The window names the mounted external volumes on the first run, and says
      so when none is connected. The folder picker is the system panel, which
      lists every mounted drive beside the internal one.

## 4. Failure modes

| Situation | Result |
|---|---|
| Target on exFAT, NTFS, or an unknown filesystem | Preflight error, nothing copied |
| Target lacks free space (needs size × 1.1) | Preflight error, nothing copied |
| Tool is running | Preflight error, nothing copied |
| Process table unreadable | Preflight error, nothing copied |
| Environment variable pins the cache path | Preflight error, nothing copied |
| Tool does not answer its path query | Item shows Blocked, nothing copied |
| Folder must stay on this Mac (simulator devices) | Item shows Blocked with the reason, nothing copied |
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
| Keep-local without a record | Error, nothing changed |
| Keep-local while the original path is a symlink | Error, nothing changed, record kept |
| Keep-local while the original path is missing | Error, nothing changed, record kept |
| Keep-local while the drive is away | Error, nothing changed, record kept |
| Keep-local with a deleted copy and a reachable drive | Copy stays absent, record removed |
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
- The right gauge counts the caches whose copy sits under the chosen folder.
  The ledger keeps each cache's own destination, so caches can sit on several
  drives, and another drive never adds to the folder on screen.
- A cache is a card, and the card keeps every fact visible: checkbox, title,
  size, state chip, folder, copy, the note that names the change Exodrive makes,
  the failure with its next step, and the one action that fits the state. No
  fact waits behind a press.
- The state chip carries a word, so color is never the only signal:
  `On this Mac`, `On the drive`, `Cannot move`, `Unused`, `Rewritten`,
  `Move failed`, `Move back failed`.
- `stateOf` reads the rewritten link first, then the move, then use, then the
  block. A moved cache shows where it lives even when it also carries a reason.
- A cache that is moved, unused, or blocked shows a disabled checkbox.
  `movable` is the only gate that select all, the move, and the count of caches
  that can move read.
- A card offers the action that fits its place. A local cache offers `Move`,
  which asks for a confirmation that holds that cache alone; without a checked
  folder it opens the folder picker first. A moved cache offers `Check the copy`
  and `Move back`. An unused cache offers `Delete cache`. A cache that a tool
  wrote again offers `Keep the local folder`, because a move back would fail on
  the two trees. Each one asks for confirmation. The delete names the path and
  says the data is gone.
- The rail lists what the check found, by group: Node, Python, Ruby, Rust,
  Go, Apple, JVM, Mobile, Homebrew, Docker, and Other. Each group shows its count
  and a bar split by place, so a press on a group reads before it filters. A
  group with no cache is absent. `familyLabel` returns the group name itself,
  so Apple, JVM, Mobile, and Homebrew read the same in both languages.
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
- The folder picker is the system's: `Scope.PickDirectories` opens the
  platform panel (Finder's `NSOpenPanel` on macOS), which lists every mounted
  drive, keeps its own sidebar and scrolling, and needs no folder of the
  window's own. The window asks for one folder. A cancelled pick changes
  nothing, and a pick that fails shows its reason in the notice banner.
- A picked folder is checked exactly as a typed path is. The field shows the
  path either way, so the person can edit it afterwards.
- The window reads the login shell PATH before the first scan. A window opened
  from Finder or the Dock inherits the launchd PATH, which holds no Homebrew,
  nvm, or custom folder. Without that step, every tool query fails and each
  cache shows `Cannot move`.
- The window sets its own title through `desktop.SetTitle` before the first
  frame, because `nsgo.Run` sets none. `WindowTitle` lives in
  `desktop/logo.go`.

### Language

- `desktop/text.go` holds two catalogs, `englishText` and `chineseText`, as one
  struct type. A field that one catalog lacks does not compile, so a new
  string cannot ship in one language only.
- `languageOf` picks the catalog: `EXODRIVE_LANG` first (`zh`, `zh-Hans`, or
  `zh-*` give Chinese), then `cx.Env().Locale()`. No other value changes it.
- The window owns only the text it writes: labels, buttons, notices, dialogs,
  and the two sentences inside `TargetError`. A provider message, a filesystem
  error, and a tool's own output stay as written, because the reason matters
  more than the language.
- `text_test.go` reflects over both catalogs and fails when a field is empty,
  when a Chinese field equals its English twin, or when a window renders
  English while the language is Chinese.

### First run

- `AppState.WelcomeOpen` starts true unless `StatePath("welcome")` exists.
  The card lists three numbered steps and two buttons: `Skip` and `Choose
  folder`.
- `markWelcomeSeen` writes the marker. A folder that passes `Engine.CheckTarget`,
  or a press on `Skip`, closes the card and writes the marker. A failed check
  keeps the card open.
- The card sits above the drive panel, so a person reads the steps before the
  field. It never blocks a press on anything below it.

### Stop and quit

- nsgo exposes no quit hook, so the window cannot hold a quit open. The
  specification does not promise one.
- A move, a move back, and a delete write `inflight.json` through
  `WriteInFlight` before they start. Each one calls `ClearInFlight` in a
  `defer`, so the file is present only while a change runs.
- The next window start reads that file through `interruptedNotice` and shows
  the cache that was changing. It then deletes the file, so the notice appears
  once.
- A record with no ID, or one that does not parse, is ignored: `ReadInFlight`
  reports `false` and the window stays quiet. A stale file must never stop a
  start.
- This record warns. It does not repair. A window that closed mid-copy leaves a
  partial copy on the drive and the full cache on this Mac. The next check
  reads the ledger and shows the cache as `On this Mac`.

### Update check

- `checkForUpdate` reads the public release list from GitHub once per start, on
  a background goroutine, and posts the result to the UI thread.
- A newer tag puts one line in the footer with a button that opens the release
  page through `cx.OpenURL`. Exodrive never downloads or replaces its own
  bundle: an ad-hoc signed app cannot swap itself safely.
- `EXODRIVE_NO_UPDATE_CHECK=1` turns the check off. A missing network, a
  non-200 reply, or a tag that does not parse leaves the footer without the
  line. The check never shows a failure.
- `desktop.Version` comes from `-ldflags -X main.version`; a development build
  reads `dev` and never sees an update.

## 6. Command line contract

- `check` prints the detected caches, the tool version, and the switch method.
  A migrated cache prints its copy as `   └── copy: <path>`. A cache that a
  tool wrote again prints the state `Rewritten` and the next step on the line
  below. Its bytes count as local.
  A provider that fails prints a warning; the other items stay usable. An
  unreadable ledger stops the command.
- A scan measures every cache it finds. `DirSize` reads up to eight folders at
  once, and every call in one run shares those readers, so a 60 GB store does
  not open hundreds of reads at the same time.
- `preflight <item> --target` runs the same checks a migration runs first.
- `migrate --target` moves the selected caches. The command prints a blocked
  cache as `[blocked] <title>: <reason>` and leaves it out of the run. When
  every selected cache is blocked, it exits 1. `--dry-run` runs every check
  and prints the plan. It copies no data, switches no tool, and writes no
  record. A blocked item makes the command exit 1.
- A move prints the copy progress bar, then `Checking the copy...` with a
  second bar. Both bars read the same source size.
- `rollback <item>` restores one cache and removes its record.
- `verify <item>` and `verify --all` check moved copies against their recorded
  digests. Each record prints as `[state] <id>: <copy>`, and a check that read
  the copy adds `(<files>, <size>)`. A state other than `ok` adds its next step
  on the line below. The command exits 1 when any record is not `ok`. `--json`
  writes the results to stdout and nothing else; a message goes to stderr.
- `orphans [--clean]` lists or deletes abandoned caches.
- `version` prints the build version. A release build shows its number with a
  `v` prefix; a development build shows its commit or `dev`.

## 7. Release contract

- `release.yml` runs the test suite, builds six CLI targets, builds four
  Windows programs and two app bundles, writes `SHA256SUMS`, and publishes the
  release.
- The matrix builds `darwin` and `linux` for `arm64` and `amd64`, and `windows`
  for `amd64` and `arm64`. Every target builds on `ubuntu-latest` with
  `CGO_ENABLED=0`.
- A Windows program carries `.exe`. A Windows archive is
  `exodrive_<tag>_windows_<arch>.zip`, holding `exodrive.exe`,
  `exodrive-gui.exe`, the READMEs, and the license. Windows users open a zip
  with one click, so the archive is a zip and not a tarball.
- The CI bundle is ad-hoc signed, because the workflow holds no signing
  identity. `make app` with `SIGN_IDENTITY` produces a Developer ID build.
- A Developer ID build carries `--options runtime` and `--timestamp`.
  Notarization refuses a bundle without both, so the two paths cannot mix.
- `make notarize` needs `NOTARY_PROFILE`. `scripts/notarize.sh` stops when the
  profile is missing, when the signature is not a Developer ID, when Apple
  rejects the upload, or when Gatekeeper still rejects the stapled bundle.
- A bundle that carries no ticket keeps the Gatekeeper prompt. It must not ship
  as a notarized build.
- The release build passes `-X main.version=${VERSION}` to both the command
  line tool and the window. The window reports that value to the update check.
- `SHA256SUMS` names each asset without a path prefix, so a script matches one
  line. Releases before v0.8.0 wrote `./name`; the install script reads both.

## 8. Pages contract

- `site/` holds four pages: `index.html`, `zh/index.html`, `docs/index.html`,
  and `zh/docs/index.html`. They have no build step.
- `scripts/check-site.sh` is the build: each page needs `lang`, `title`, and
  `viewport`, each relative link must resolve, and each page needs its twin in
  the other language. CI runs it in the test job, and `pages.yml` runs it again
  before it uploads.
- A page in one language that names a page in the other language uses a
  relative link, so the pair works under any base path. A link that names a
  file the page does not ship, such as `install.sh`, uses the full site URL.
- `pages.yml` publishes `site/` to GitHub Pages on a push to `main` that
  touches a page. The repository needs the Pages source set to GitHub Actions.

### The install script

- `scripts/install.sh` is the one-line install:
  `curl -fsSL https://jungley8.github.io/exodrive/install.sh | sh`.
- `pages.yml` copies it to `site/install.sh` before it uploads, so `site/` holds
  no second copy in Git. It copies `scripts/install.ps1` the same way.
- The script reads the newest tag from the redirect of
  `/releases/latest`, picks `exodrive_<tag>_<os>_<arch>.tar.gz` or
  `Exodrive_<tag>_darwin_<arch>.app.zip`, and fetches with `curl` or `wget`.
- It reads the one line of `SHA256SUMS` that names the asset and checks it with
  `shasum` or `sha256sum`. A download that fails, a line that is missing, or a
  hash that differs stops the script before it writes a file.
- It installs the command line tool to `~/.local/bin` and the app bundle to
  `~/Applications`. `--cli`, `--app`, `--version <tag>`, and `--dir <folder>`
  change that, and an unknown option exits 2.
- It never removes the quarantine flag. A browser download keeps the Gatekeeper
  prompt, and `curl` adds no flag, so the two paths differ and the script says
  which one applies.
- CI runs `sh -n scripts/install.sh`, so a syntax error fails the build.

### The Windows install script

- `scripts/install.ps1` is the one-line install:
  `irm https://jungley8.github.io/exodrive/install.ps1 | iex`.
- It reads the newest tag from the redirect of `/releases/latest`, picks
  `exodrive_<tag>_windows_<arch>.zip`, and reads `PROCESSOR_ARCHITECTURE` to
  choose `amd64` or `arm64`.
- It checks the archive with `Get-FileHash` against the one line of
  `SHA256SUMS` that names the asset. A missing line or a different hash stops
  the script before it writes a file.
- It installs both programs to `%LOCALAPPDATA%\Programs\Exodrive`.
  `-Version <tag>` and `-Dir <folder>` change that. It prints the command that
  adds the folder to PATH; it does not change PATH itself.
- CI parses the file with the PowerShell parser, so a syntax error fails the
  build.

## 9. Failure log contract

Exodrive keeps a local record of every failure it shows. Nothing leaves the
machine unless a person opens an issue.

- The log is `StatePath("failures.jsonl")`, beside the ledger. It holds one
  JSON object per line, mode 0600.
- A record holds `time` (RFC 3339, UTC), `version`, `os`, `arch`, `action`,
  `kind`, `item`, and `message`.
- `action` names the step that stopped: `scan`, `choose`, `move`, `move back`,
  `delete`, `check`, `preflight`, `migrate`, `rollback`, or `orphans`.
- `kind` is one stable token from `FailureKind` in
  `internal/core/failure_kind.go`: `target`, `unsupported`, `space`,
  `running`, `symlink`, `verification`, `env`, `no record`, `ledger`,
  `differs`, `missing`, or `other`. The window's `explain` and the log read
  the same function, so the two cannot drift.
- `message` is the error text with the home folder replaced by `~`, cut at 600
  bytes on a UTF-8 boundary. When the home folder is unknown, the message is
  dropped and the record keeps its kind.
- The file stops at 256 KB. A write past that point keeps the newest whole
  lines, about half the cap, and drops the rest. The write goes through a
  temporary file and a rename, under a lock of its own.
- `RecordFailure` is best effort. A caller ignores its error, so a full disk, a
  read-only folder, or a lock that fails never changes the result of a move, a
  move back, or a delete.
- `RecentFailures` reads the newest records, oldest first. A line that does not
  parse is skipped, and a missing file reads as no record. Reading never fails
  a caller.
- The folder field runs a check on every edit. That check writes no record,
  because a half-typed path is not a failure. The move that follows records the
  same reason once.
- `EXODRIVE_NO_FAILURE_LOG` stops every write. A non-empty value turns the log
  off; the window still shows each reason.

### The report

- `recordFailure` writes the record, and `shownFailure` keeps the same cause in
  memory, so a card can copy its own details.
- **Copy details** on a failed card, and in the report dialog, calls
  `Scope.SetClipboard`.
- The footer's **Report a problem** reads the newest five records once and
  holds the text in `AppState.ReportText`. The dialog shows exactly that text.
- **Open a GitHub issue** calls `Scope.OpenURL` with
  `https://github.com/Jungley8/exodrive/issues/new`, the newest failure as the
  title, and the text as the body. The body stops at 2000 bytes on a line
  boundary. Exodrive sends nothing itself and holds no network client for a
  report.
- `exodrive doctor` prints the version, both paths, and the newest ten
  records, with the home folder redacted. It exits 0 and sends nothing.

## 10. Ledger integrity

- `Manifest.Version` is 2. A version 1 ledger loads, and each of its records
  reports as `unverified`; the next save writes version 2. A ledger with a
  version above 2 stops the load with a message that names the newer build,
  because an older build cannot write fields it does not know.
- A record holds `tree_digest`, `file_count`, and `total_bytes`. The digest
  covers the copy only. Each file enters as
  `file\0<rel>\0<size>\0<sha256>\n`, in sorted relative-path order, then each
  link as `link\0<rel>\0<target>\n`. One SHA-256 covers the stream. Two copies of
  one tree under different paths give the same digest.
- A migration computes the digest in the pass that already reads both trees
  (`VerifyTreeSummary`, `VerifyFileSummary`), so a move pays no second read.
  `DigestPath` reads one path the same way for a later check.
- `Engine.Verify` returns `ok`, `changed`, `missing`, or `unverified`. A record
  with no digest is `unverified` and never `ok`. A path that does not exist is
  `missing`. Any other read error returns an error, so no caller can call the
  copy good.
- `Verify` writes nothing: no record, no file on the drive, no in-flight mark.
- `Engine.VerifyAll` reads one snapshot of the ledger and checks each record in
  ID order.
- The window shows **Check the copy** on a card whose record names a copy. The
  answer stays on the card. A check that stops shows a banner and no failure
  block: the block's **Try again** moves data, and a check moves none.
- A blocked item writes no record. The move stops in preflight, so the ledger
  holds no entry for an item that never moved.
