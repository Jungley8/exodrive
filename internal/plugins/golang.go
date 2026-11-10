package plugins

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"exodrive/internal/core"
	"exodrive/internal/fsutil"
)

type GoProvider struct{}

func NewGoProvider() *GoProvider {
	return &GoProvider{}
}

func (p *GoProvider) Name() string {
	return "go"
}

func (p *GoProvider) queryGoEnv(ctx context.Context, varName string) string {
	cmd := exec.CommandContext(ctx, "go", "env", varName)
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err == nil {
		return strings.TrimSpace(out.String())
	}
	return ""
}

func (p *GoProvider) Detect(ctx context.Context) ([]core.CacheItem, error) {
	var items []core.CacheItem

	gocache := p.queryGoEnv(ctx, "GOCACHE")
	gomodcache := p.queryGoEnv(ctx, "GOMODCACHE")

	targets := []struct {
		id       string
		title    string
		sub      string
		curr     string
		category core.CacheCategory
	}{
		{
			id:       "go.build",
			title:    "Go Build Cache (GOCACHE)",
			sub:      "go-build",
			curr:     gocache,
			category: core.CategoryBuildCache,
		},
		{
			id:       "go.mod",
			title:    "Go Module Cache (GOMODCACHE)",
			sub:      filepath.Join("go", "pkg", "mod"),
			curr:     gomodcache,
			category: core.CategoryPackageStore,
		},
	}

	for _, t := range targets {
		if t.curr == "" {
			continue
		}
		if fi, err := os.Stat(t.curr); err == nil && fi.IsDir() {
			size, _ := fsutil.DirSize(ctx, t.curr)
			isSymlink := fsutil.IsSymlink(t.curr)
			isMigrated := strings.HasPrefix(t.curr, "/Volumes/") || isSymlink

			items = append(items, core.CacheItem{
				ID:             t.id,
				ToolName:       "go",
				Title:          t.title,
				Category:       t.category,
				CurrentPath:    t.curr,
				ConfiguredPath: t.curr,
				SizeBytes:      size,
				IsActive:       true,
				IsMigrated:     isMigrated,
				IsSymlink:      isSymlink,
				RedirectMethod: core.RedirectNativeConfig,
				TargetSubdir:   t.sub,
			})
		}
	}

	return items, nil
}

func (p *GoProvider) PreflightCheck(ctx context.Context, item core.CacheItem, targetRoot string) error {
	running, err := fsutil.CheckRunningProcesses("go")
	if err != nil {
		return err
	}
	if len(running) > 0 {
		return fmt.Errorf("go tool is running (PID: %s): stop active go processes before migration", running[0].PID)
	}
	if item.ID == "go.mod" {
		return envOverride("GOMODCACHE")
	}
	return envOverride("GOCACHE")
}

func (p *GoProvider) Migrate(ctx context.Context, item core.CacheItem, targetRoot string, progress fsutil.MigrateProgress) error {
	dest := filepath.Join(targetRoot, item.TargetSubdir)
	if err := os.MkdirAll(dest, 0755); err != nil {
		return err
	}

	if err := fsutil.CopyDir(item.CurrentPath, dest, item.SizeBytes, progress.Copy); err != nil {
		return fmt.Errorf("copy %s: %w", item.Title, err)
	}
	if err := fsutil.VerifyTreeProgress(item.CurrentPath, dest, progress.Verify); err != nil {
		return fmt.Errorf("%s: %w", item.Title, err)
	}

	varName := "GOCACHE"
	if item.ID == "go.mod" {
		varName = "GOMODCACHE"
	}

	if err := core.RecordMigration(core.MigrationRecord{
		ID:             item.ID,
		ToolName:       p.Name(),
		OriginalPath:   item.CurrentPath,
		DestinationDir: dest,
		RedirectMethod: core.RedirectNativeConfig,
		OriginalConfig: item.ConfiguredPath,
		MigratedAt:     time.Now(),
		SizeBytes:      item.SizeBytes,
	}); err != nil {
		return err
	}

	cmd := exec.CommandContext(ctx, "go", "env", "-w", fmt.Sprintf("%s=%s", varName, dest))
	if out, err := cmd.CombinedOutput(); err != nil {
		return switchFailed(item.ID, fmt.Errorf("go env -w failed (%s): %w", string(out), err))
	}

	return removeLocalSource(item.CurrentPath, dest)
}

func (p *GoProvider) Rollback(ctx context.Context, item core.CacheItem) error {
	rec, err := loadRecord(item.ID)
	if err != nil {
		return err
	}

	varName := "GOCACHE"
	if item.ID == "go.mod" {
		varName = "GOMODCACHE"
	}

	if rec.OriginalConfig != "" && rec.OriginalConfig != rec.DestinationDir {
		if err := runCommand(ctx, "go", "env", "-w", fmt.Sprintf("%s=%s", varName, rec.OriginalConfig)); err != nil {
			return err
		}
	} else {
		if err := runCommand(ctx, "go", "env", "-u", varName); err != nil {
			return err
		}
	}

	if err := restoreNativeData(rec); err != nil {
		return err
	}

	return core.RemoveRecord(item.ID)
}
