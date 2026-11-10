# Contributing to Exodrive

Thank you for your interest. This document tells you how to send a change that
the project can merge.

## Code of conduct

Be direct and respectful. Discuss code, not people. Harassment or personal
attacks end the discussion.

## Sign your work (DCO)

Every commit must carry a `Signed-off-by` line. This is the Developer
Certificate of Origin, version 1.1. It states that you wrote the change, or that
you have the right to submit it.

Add the line with every commit:

```sh
git commit -s
```

The line looks like this:

```text
Signed-off-by: Your Name <you@example.com>
```

A pull request without the line does not merge. Do not add a
`Co-authored-by` trailer for a real person who did not write the change.

## Development setup

Exodrive needs Go 1.27 or later.

```sh
git clone <repository-url> exodrive
cd exodrive
make build
make test
```

The window needs macOS. The command line builds on macOS and Linux.

## Code rules

Follow these rules for every change.

- Run `gofmt -w .` before you commit. CI rejects a file that `gofmt -d .` reports.
- Run `go vet ./...`. CI rejects a warning.
- Run `golangci-lint run ./...`. CI rejects a warning.
- Write user-facing text in Simplified Technical English. Use one instruction
  per sentence. Keep each sentence at 20 words or less. Use the active voice.
- Name a concept with one word in the whole user interface. Use "move" for a
  migration and "rollback" for the reverse.
- Do not add a fallback that hides a failure. Return an error.

## Safety rules for code

Data loss is the main risk in this project. Every change must keep these
invariants.

1. Never delete a local cache before the destination copy passes
   `fsutil.VerifyTree`.
2. Never point a tool at a path that does not hold its data.
3. Write the ledger record before the switch. Remove it when the switch fails.
4. Never remove a ledger record before the rollback restores the data. A
   rollback that cannot restore the data keeps the record.
5. Never swallow an error from a copy, a configuration command, or a rename.
6. Refuse an unknown filesystem, an unknown provider, and an unknown redirect.
7. Refuse a migration while an environment variable pins the cache path.

A pull request that breaks an invariant does not merge.

## Tests

Add a test for every behavior change. The test must fail without your change.

- Provider changes: add a case to `internal/plugins/lifecycle_test.go`. Use the
  mock environment so the test never touches a real cache.
- Engine changes: add a case to `internal/core/engine_test.go`.
- Filesystem changes: add a case to `internal/fsutil/*_test.go`.
- Window changes: add a case to `desktop/app_test.go`. Mount the window with
  `nsgotest` and press the real controls.

Run the full suite before you open a pull request:

```sh
make check
```

## Pull requests

1. Branch from `main`. Name the branch for the change, for example
   `fix/rollback-missing-destination`.
2. Keep one pull request to one vertical slice. Do not mix a refactor with a
   behavior change.
3. Describe the user path and the evidence. Include the command you ran and its
   output. A screenshot helps for a window change.
4. Write the commit subject in the imperative mood: `fix: keep the record when
   the destination is missing`.
5. Sign every commit with `-s`.
6. Wait for CI. A red check blocks the merge.

## Adding a toolchain provider

A provider implements `plugins.Provider`:

```go
type Provider interface {
    Name() string
    Detect(ctx context.Context) ([]core.CacheItem, error)
    PreflightCheck(ctx context.Context, item core.CacheItem, targetRoot string) error
    Migrate(ctx context.Context, item core.CacheItem, targetRoot string, onProgress fsutil.CopyProgressCallback) error
    Rollback(ctx context.Context, item core.CacheItem) error
}
```

Add these items:

- A unique `ID` for each cache the provider reports.
- A `Detect` that reads the tool's own configuration, not a guess.
- A `PreflightCheck` that stops a running tool.
- A `Migrate` that copies, calls `fsutil.VerifyTree`, records the move, switches
  the tool, and only then removes the local copy.
- A `Rollback` that restores the configuration and the data, and that keeps the
  record when either step fails.
- A lifecycle test with a mock executable, as the existing providers have.
- A row in the README table.

## Release

A maintainer creates the tag. CI builds the binaries and publishes them.

```sh
git tag -a v0.1.0 -m "v0.1.0"
git push origin v0.1.0
```

## License

By contributing, you agree that your contribution is licensed under the Apache
License 2.0.
