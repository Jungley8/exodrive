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
	fmt.Println("\n==========================================================================================")
	fmt.Printf("%-18s %-12s %-10s %-10s %-32s\n", "TOOL / CACHE", "CATEGORY", "SIZE", "STATUS", "LOCATION")
	fmt.Println("------------------------------------------------------------------------------------------")

	var totalInternal, totalMigrated int64

	for _, item := range items {
		status := "Internal"
		if item.IsMigrated {
			status = "Migrated"
			totalMigrated += item.SizeBytes
		} else {
			totalInternal += item.SizeBytes
		}

		if !item.IsActive {
			status = "Abandoned"
		}

		shortPath := item.CurrentPath
		if len(shortPath) > 36 {
			shortPath = "..." + shortPath[len(shortPath)-33:]
		}

		fmt.Printf("%-18s %-12s %-10s %-10s %-32s\n",
			truncate(item.Title, 17),
			item.Category,
			FormatBytes(item.SizeBytes),
			status,
			shortPath,
		)
		if item.Warning != "" {
			fmt.Printf("   └── [!] %s\n", item.Warning)
		}
	}

	fmt.Println("------------------------------------------------------------------------------------------")
	fmt.Printf("Total Local SSD Cache: %s | Already on External: %s\n", FormatBytes(totalInternal), FormatBytes(totalMigrated))
	fmt.Println("==========================================================================================")
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
