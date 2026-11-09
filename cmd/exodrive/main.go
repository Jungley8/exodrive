package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"exodrive/internal/core"
	"exodrive/internal/plugins"
	"exodrive/internal/ui"
)

func main() {
	if len(os.Args) < 2 {
		runCheck()
		return
	}

	cmd := os.Args[1]
	switch cmd {
	case "check", "scan", "list":
		runCheck()
	case "migrate":
		runMigrate(os.Args[2:])
	case "orphans":
		runOrphans(os.Args[2:])
	case "rollback":
		runRollback(os.Args[2:])
	case "help", "-h", "--help":
		printHelp()
	default:
		fmt.Printf("Unknown command '%s'. Run 'exodrive help' for usage.\n", cmd)
		os.Exit(1)
	}
}

func printHelp() {
	fmt.Println(`Exodrive — Safe Developer Cache Offloader

Usage:
  exodrive check                     Scan and list all detected developer caches
  exodrive migrate --target=<dir>    Migrate caches to external storage volume
  exodrive orphans [--clean]         Detect or clean abandoned legacy caches
  exodrive rollback <item_id>        Restore a migrated cache to internal storage
  exodrive help                      Display this help message

Options for migrate:
  --target   Destination directory on external APFS/ext4 drive (Required)
  --tools    Comma-separated list of tools to migrate (e.g. npm,pnpm,go)
  --all      Migrate all eligible local caches`)
}

func buildEngine() *core.Engine {
	reg := plugins.NewRegistry()
	var accessors []core.ProviderAccessor
	for _, p := range reg.All() {
		accessors = append(accessors, p)
	}
	return core.NewEngine(accessors...)
}

func runCheck() {
	engine := buildEngine()
	fmt.Println("Scanning developer caches across 10 toolchains (npm, pnpm, yarn, cargo, go, bun, uv, pip, docker, orphans)...")
	ctx := context.Background()
	items, err := engine.ScanAll(ctx)
	if err != nil {
		fmt.Printf("Error scanning: %v\n", err)
		return
	}
	ui.RenderTable(items)
}

func runMigrate(args []string) {
	fs := flag.NewFlagSet("migrate", flag.ExitOnError)
	target := fs.String("target", "", "Destination directory on external storage (e.g. /Volumes/PHD/exodrive-caches)")
	tools := fs.String("tools", "", "Comma-separated list of tools to migrate")
	all := fs.Bool("all", false, "Migrate all unmigrated caches")
	_ = fs.Parse(args)

	if *target == "" {
		fmt.Println("Error: --target is required. Specify external volume path (e.g. /Volumes/PHD/exodrive-caches).")
		os.Exit(1)
	}

	engine := buildEngine()
	ctx := context.Background()
	items, err := engine.ScanAll(ctx)
	if err != nil {
		fmt.Printf("Scan failed: %v\n", err)
		os.Exit(1)
	}

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

	fmt.Printf("\nTarget external directory: %s\n", *target)
	fmt.Printf("Selected %d cache target(s) for migration.\n\n", len(candidates))

	for _, it := range candidates {
		fmt.Printf("==> Migrating %s (%s, %s)...\n", it.Title, it.CurrentPath, ui.FormatBytes(it.SizeBytes))

		err := engine.Migrate(ctx, it, *target, func(copied, total int64) {
			ui.RenderProgressBar(it.ToolName, copied, total)
		})
		fmt.Println()

		if err != nil {
			fmt.Printf("❌ Migration failed for %s: %v\n\n", it.Title, err)
		} else {
			fmt.Printf("✅ %s migrated successfully.\n\n", it.Title)
		}
	}

	fmt.Println("Migration completed. Ledger updated at ~/.exodrive/manifest.json")
}

func runOrphans(args []string) {
	fs := flag.NewFlagSet("orphans", flag.ExitOnError)
	clean := fs.Bool("clean", false, "Permanently delete detected abandoned caches")
	_ = fs.Parse(args)

	engine := buildEngine()
	ctx := context.Background()
	items, err := engine.ScanAll(ctx)
	if err != nil {
		fmt.Printf("Scan error: %v\n", err)
		return
	}

	var orphans []core.CacheItem
	for _, it := range items {
		if it.Category == core.CategoryOrphan {
			orphans = append(orphans, it)
		}
	}

	if len(orphans) == 0 {
		fmt.Println("No abandoned or orphaned developer caches found.")
		return
	}

	fmt.Printf("\nFound %d abandoned cache(s):\n", len(orphans))
	for _, o := range orphans {
		fmt.Printf(" - [%s] %s (%s)\n", o.ID, o.CurrentPath, ui.FormatBytes(o.SizeBytes))
		fmt.Printf("   Note: %s\n", o.Warning)
	}

	if *clean {
		fmt.Println("\nCleaning abandoned caches...")
		for _, o := range orphans {
			fmt.Printf("Removing %s... ", o.CurrentPath)
			if err := engine.CleanOrphan(o.CurrentPath); err != nil {
				fmt.Printf("Failed: %v\n", err)
			} else {
				fmt.Printf("Done! Reclaimed %s\n", ui.FormatBytes(o.SizeBytes))
			}
		}
	} else {
		fmt.Println("\nRun 'exodrive orphans --clean' to permanently delete these abandoned caches.")
	}
}

func runRollback(args []string) {
	if len(args) < 1 {
		fmt.Println("Usage: exodrive rollback <item_id> (e.g. npm.cache, go.build)")
		return
	}
	itemID := args[0]
	engine := buildEngine()
	ctx := context.Background()

	fmt.Printf("Reverting migration for %s...\n", itemID)
	if err := engine.Rollback(ctx, itemID); err != nil {
		fmt.Printf("Rollback failed: %v\n", err)
		return
	}
	fmt.Printf("Successfully restored %s to default internal configuration.\n", itemID)
}
