package ui

import (
	"fmt"
	"strings"

	"exodrive/internal/core"
)

func FormatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

// RenderTable prints a well-aligned CLI table for the detected items.
func RenderTable(items []core.CacheItem) {
	fmt.Println("\n====================================================================================================")
	fmt.Printf("%-18s %-12s %-10s %-10s %-9s %-32s\n", "TOOL / CACHE", "CATEGORY", "SIZE", "STATUS", "METHOD", "LOCATION")
	fmt.Println("----------------------------------------------------------------------------------------------------")

	var totalInternal, totalMigrated int64

	for _, item := range items {
		// A record can hold a copy that the tool no longer reaches. A folder
		// that a tool wrote again counts as local, because it is here again.
		status := "Internal"
		switch {
		case item.LinkState == core.LinkRewritten:
			status = "Rewritten"
			totalInternal += item.SizeBytes
		case item.IsMigrated:
			status = "Migrated"
			totalMigrated += item.SizeBytes
		default:
			totalInternal += item.SizeBytes
		}

		if !item.IsActive {
			status = "Abandoned"
		}
		if item.BlockedReason != "" {
			status = "Blocked"
		}

		shortPath := item.CurrentPath
		if len(shortPath) > 36 {
			shortPath = "..." + shortPath[len(shortPath)-33:]
		}

		title := truncate(item.Title, 17)
		if item.ToolVersion != "" {
			title = truncate(item.Title+" "+item.ToolVersion, 17)
		}

		fmt.Printf("%-18s %-12s %-10s %-10s %-9s %-32s\n",
			title,
			item.Category,
			FormatBytes(item.SizeBytes),
			status,
			RedirectShortLabel(item.RedirectMethod),
			shortPath,
		)
		if item.BlockedReason != "" {
			fmt.Printf("   └── [!] %s\n", item.BlockedReason)
		}
		if item.DestinationPath != "" {
			fmt.Printf("   └── copy: %s\n", item.DestinationPath)
		}
		if item.LinkState == core.LinkRewritten {
			fmt.Println("   └── [!] A tool wrote this folder again. Keep the folder on this Mac, or restore the copy.")
		}
		if item.Warning != "" {
			fmt.Printf("   └── [!] %s\n", item.Warning)
		}
	}

	fmt.Println("----------------------------------------------------------------------------------------------------")
	fmt.Printf("Total Local SSD Cache: %s | Already on External: %s\n", FormatBytes(totalInternal), FormatBytes(totalMigrated))
	fmt.Println("====================================================================================================")
	fmt.Println()
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-2] + ".."
}

func RenderProgressBar(title string, copied, total int64) {
	if total <= 0 {
		return
	}
	percent := float64(copied) / float64(total) * 100
	if percent > 100 {
		percent = 100
	}
	barLen := 30
	completed := int(float64(barLen) * (percent / 100))
	bar := strings.Repeat("=", completed) + strings.Repeat(" ", barLen-completed)
	fmt.Printf("\r[%s] [%s] %.1f%% (%s / %s)", title, bar, percent, FormatBytes(copied), FormatBytes(total))
}

// RedirectLabel names how Exodrive points the tool at the moved data. The
// window and the command line use the same words.
func RedirectLabel(method core.RedirectMethod) string {
	switch method {
	case core.RedirectNativeConfig:
		return "configuration"
	case core.RedirectSymlink:
		return "symbolic link"
	case core.RedirectManual:
		return "manual step"
	default:
		return "unknown method"
	}
}

// RedirectShortLabel fits the method into a table column.
func RedirectShortLabel(method core.RedirectMethod) string {
	switch method {
	case core.RedirectNativeConfig:
		return "config"
	case core.RedirectSymlink:
		return "symlink"
	case core.RedirectManual:
		return "manual"
	default:
		return "unknown"
	}
}
