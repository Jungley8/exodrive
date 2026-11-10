package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"exodrive/internal/core"
	"exodrive/internal/plugins"
	"exodrive/internal/ui"
)

// version is set at build time:
// go build -ldflags "-X main.version=0.1.0" ./cmd/exodrive
var version = "dev"

func main() {
	if len(os.Args) < 2 {
		runCheck(nil)
		return
	}

	cmd := os.Args[1]
	switch cmd {
	case "check", "scan", "list", "status":
		runCheck(os.Args[2:])
	case "--json":
		runCheck([]string{"--json"})
	case "preflight":
		runPreflight(os.Args[2:])
	case "migrate":
		runMigrate(os.Args[2:])
	case "orphans":
		runOrphans(os.Args[2:])
	case "rollback":
		runRollback(os.Args[2:])
	case "version", "--version", "-v":
		printVersion()
	case "help", "-h", "--help":
		printHelp()
	default:
		fmt.Printf("Unknown command '%s'. Run 'exodrive help' for usage.\n", cmd)
		os.Exit(1)
	}
}

func printVersion() {
	fmt.Printf("Exodrive %s (%s/%s)\n", versionLabel(version), runtime.GOOS, runtime.GOARCH)
}

// versionLabel formats the build version for display. A release build carries
// a number, so it gets the "v" prefix. A development build carries a commit
// hash or "dev", and shows as it is.
func versionLabel(v string) string {
	if len(v) > 0 && v[0] >= '0' && v[0] <= '9' {
		return "v" + v
	}
	return v
}

func printHelp() {
	fmt.Println(`Exodrive — Developer Cache Migrator

Usage:
  exodrive check [--json]             Scan and list detected developer caches
  exodrive preflight <item> --target  Run safety checks before migration
  exodrive migrate --target=<dir>     Move caches to external storage
  exodrive orphans [--clean]          Detect or delete abandoned caches
  exodrive rollback <item_id>         Move a migrated cache back
  exodrive version                    Display version information
  exodrive help                       Display this help message

A migration copies the cache, checks the copy, then removes the local copy.
A rollback copies the cache back and removes the external copy.

Options for check:
  --json     Output scan results as formatted JSON

Options for migrate:
  --target   Destination directory on external storage (Required)
  --tools    Comma-separated list of tools to migrate (e.g. npm,pnpm,go)
  --all      Migrate all eligible local caches
  --dry-run  Run every check and print the plan without moving data`)
}

func buildEngine() *core.Engine {
	reg := plugins.NewRegistry()
	var accessors []core.ProviderAccessor
	for _, p := range reg.All() {
		accessors = append(accessors, p)
	}
	return core.NewEngine(accessors...)
}

// scanCaches returns every detected cache. It stops when the ledger cannot be
// read. A toolchain that fails to detect prints a warning, and the other items
// stay usable.
func scanCaches(engine *core.Engine) []core.CacheItem {
	items, err := engine.ScanAll(context.Background())
	if err == nil {
		return items
	}
	if errors.Is(err, core.ErrLedger) {
		fmt.Printf("Scan failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "Warning: some toolchains could not be detected: %v\n", err)
	return items
}

func runCheck(args []string) {
	fs := flag.NewFlagSet("check", flag.ExitOnError)
	jsonOutput := fs.Bool("json", false, "Output results in JSON format")
	_ = fs.Parse(args)

	engine := buildEngine()
	items := scanCaches(engine)

	if *jsonOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(items); err != nil {
			fmt.Printf("JSON encode failed: %v\n", err)
			os.Exit(1)
		}
		return
	}

	fmt.Println("Scanning developer caches across supported toolchains...")
	ui.RenderTable(items)
}

func runPreflight(args []string) {
	fs := flag.NewFlagSet("preflight", flag.ContinueOnError)
	target := fs.String("target", "", "Destination directory on external storage (e.g. /Volumes/ExternalDrive/exodrive-caches)")

	var itemID string
	var flagArgs []string
	for _, arg := range args {
		if strings.HasPrefix(arg, "-") {
			flagArgs = append(flagArgs, arg)
		} else if itemID == "" {
			itemID = arg
		}
	}
	_ = fs.Parse(flagArgs)

	if itemID == "" {
		fmt.Println("Usage: exodrive preflight <item_id> --target=<dir>")
		os.Exit(1)
	}

	if *target == "" {
		fmt.Println("Error: --target is required. Specify external volume path (e.g. /Volumes/ExternalDrive/exodrive-caches).")
		os.Exit(1)
	}

	engine := buildEngine()
	ctx := context.Background()
	items := scanCaches(engine)

	var targetItem *core.CacheItem
	for _, it := range items {
		if it.ID == itemID {
			targetItem = &it
			break
		}
	}

	if targetItem == nil {
		fmt.Printf("Error: cache item '%s' not found.\n", itemID)
		os.Exit(1)
	}

	if err := engine.Preflight(ctx, *targetItem, *target); err != nil {
		fmt.Printf("Preflight failed for %s: %v\n", itemID, err)
		os.Exit(1)
	}

	fmt.Printf("Preflight passed for %s on destination %s.\n", itemID, *target)
}

func runMigrate(args []string) {
	fs := flag.NewFlagSet("migrate", flag.ExitOnError)
	target := fs.String("target", "", "Destination directory on external storage (e.g. /Volumes/ExternalDrive/exodrive-caches)")
	tools := fs.String("tools", "", "Comma-separated list of tools to migrate")
	all := fs.Bool("all", false, "Migrate all unmigrated caches")
	dryRun := fs.Bool("dry-run", false, "Check every selected cache without moving data")
	_ = fs.Parse(args)

	if *target == "" {
		fmt.Println("Error: --target is required. Specify external volume path (e.g. /Volumes/ExternalDrive/exodrive-caches).")
		os.Exit(1)
	}

	engine := buildEngine()
	ctx := context.Background()
	items := scanCaches(engine)

	toolFilter := make(map[string]bool)
	if *tools != "" {
		for _, t := range strings.Split(*tools, ",") {
			toolFilter[strings.TrimSpace(t)] = true
		}
	}

	var candidates []core.CacheItem
	for _, it := range items {
		if it.IsMigrated {
			continue // Already migrated
		}
		if !it.IsActive && it.Category == core.CategoryOrphan {
			continue // Use orphans command for dead caches
		}
		if *all || toolFilter[it.ToolName] || toolFilter[it.ID] {
			candidates = append(candidates, it)
		}
	}

	if len(candidates) == 0 {
		fmt.Println("No matching unmigrated caches found. Run 'exodrive check' to inspect items.")
		return
	}

	if *dryRun {
		runMigrateDryRun(ctx, engine, candidates, *target)
		return
	}

	fmt.Printf("\nTarget external directory: %s\n", *target)
	fmt.Printf("Selected %d cache target(s) for migration.\n\n", len(candidates))

	var failureCount int
	var successCount int
	var reclaimedBytes int64

	for _, it := range candidates {
		fmt.Printf("==> Migrating %s (%s, %s)...\n", it.Title, it.CurrentPath, ui.FormatBytes(it.SizeBytes))

		err := engine.Migrate(ctx, it, *target, func(copied, total int64) {
			ui.RenderProgressBar(it.ToolName, copied, total)
		})
		fmt.Println()

		if err != nil {
			fmt.Printf("❌ Migration failed for %s: %v\n\n", it.Title, err)
			failureCount++
		} else {
			fmt.Printf("✅ Migrated %s.\n\n", it.Title)
			successCount++
			reclaimedBytes += it.SizeBytes
		}
	}

	manifestPath := core.GetManifestPath()
	if failureCount > 0 {
		fmt.Printf("Migration completed with %d failure(s). Migrated %d/%d caches.\n", failureCount, successCount, len(candidates))
		fmt.Printf("Ledger updated at %s.\n", manifestPath)
		os.Exit(1)
	}

	fmt.Printf("Migration completed. Reclaimed %s on the local disk.\n", ui.FormatBytes(reclaimedBytes))
	fmt.Printf("Ledger updated at %s.\n", manifestPath)
}

// runMigrateDryRun reports what a migration would do. It runs the same checks
// and changes nothing: no copy, no switch, no ledger record.
func runMigrateDryRun(ctx context.Context, engine *core.Engine, candidates []core.CacheItem, target string) {
	fmt.Printf("\nTarget external directory: %s\n", target)
	fmt.Printf("Dry run. %d cache(s) selected. No data moves.\n\n", len(candidates))

	var reclaimable int64
	var failures int
	for _, it := range candidates {
		if err := engine.Preflight(ctx, it, target); err != nil {
			fmt.Printf("[blocked] %s: %v\n", it.Title, err)
			failures++
			continue
		}
		destination := filepath.Join(target, it.TargetSubdir)
		fmt.Printf("[ready] %s: %s -> %s (%s)\n", it.Title, it.CurrentPath, destination, ui.FormatBytes(it.SizeBytes))
		reclaimable += it.SizeBytes
	}

	fmt.Printf("\nDry run complete. %d cache(s) ready. %s would be free on the local disk.\n",
		len(candidates)-failures, ui.FormatBytes(reclaimable))
	if failures > 0 {
		os.Exit(1)
	}
}

func runOrphans(args []string) {
	fs := flag.NewFlagSet("orphans", flag.ExitOnError)
	clean := fs.Bool("clean", false, "Delete detected abandoned caches")
	_ = fs.Parse(args)

	engine := buildEngine()
	items := scanCaches(engine)

	var orphans []core.CacheItem
	for _, it := range items {
		if it.Category == core.CategoryOrphan {
			orphans = append(orphans, it)
		}
	}

	if len(orphans) == 0 {
		fmt.Println("No abandoned caches found.")
		return
	}

	fmt.Printf("\nFound %d abandoned cache(s):\n", len(orphans))
	for _, o := range orphans {
		fmt.Printf(" - [%s] %s (%s)\n", o.ID, o.CurrentPath, ui.FormatBytes(o.SizeBytes))
		fmt.Printf("   Note: %s\n", o.Warning)
	}

	if *clean {
		fmt.Println("\nDeleting abandoned caches...")
		var failCount int
		for _, o := range orphans {
			fmt.Printf("Removing %s... ", o.CurrentPath)
			if err := engine.CleanOrphan(o.CurrentPath); err != nil {
				fmt.Printf("Failed: %v\n", err)
				failCount++
			} else {
				fmt.Printf("Reclaimed %s.\n", ui.FormatBytes(o.SizeBytes))
			}
		}
		if failCount > 0 {
			os.Exit(1)
		}
	} else {
		fmt.Println("\nRun 'exodrive orphans --clean' to delete abandoned caches.")
	}
}

func runRollback(args []string) {
	if len(args) < 1 {
		fmt.Println("Usage: exodrive rollback <item_id> (e.g. npm.cache, go.build)")
		os.Exit(1)
	}
	itemID := args[0]
	engine := buildEngine()
	ctx := context.Background()

	fmt.Printf("Moving %s back to its original location...\n", itemID)
	if err := engine.Rollback(ctx, itemID); err != nil {
		fmt.Printf("Rollback failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Restored %s. The ledger record is removed.\n", itemID)
}
