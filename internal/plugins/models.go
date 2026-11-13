package plugins

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"exodrive/internal/core"
	"exodrive/internal/fsutil"
)

// modelStore describes one AI model cache. Each tool reads its models from one
// folder that grows to tens or hundreds of gigabytes, so Exodrive moves the
// folder and a link takes its place.
type modelStore struct {
	tool    string
	id      string
	title   string
	env     string // environment variable that names the root, or ""
	envSub  string // folder inside the root that holds the models
	deflt   string // folder under the home folder when the variable is empty
	subdir  string // folder under the target volume
	lock    []string
	warning string
}

// modelStores lists the AI model caches that Exodrive moves.
//
// Exodrive never rewrites these environment variables. A link replaces the path
// itself, so a variable that names the old path keeps working. The
// environment-override guard applies to a native configuration switch only,
// where the variable would win over the new setting.
var modelStores = []modelStore{
	{
		tool:    "ollama",
		id:      "ollama.models",
		title:   "Ollama Models",
		env:     "OLLAMA_MODELS",
		deflt:   ".ollama/models",
		subdir:  filepath.Join("ollama", "models"),
		lock:    []string{"ollama"},
		warning: "Stop ollama before the move.",
	},
	{
		tool:    "huggingface",
		id:      "huggingface.models",
		title:   "Hugging Face Models",
		env:     "HF_HOME",
		envSub:  "hub",
		deflt:   filepath.Join(".cache", "huggingface", "hub"),
		subdir:  filepath.Join("huggingface", "hub"),
		lock:    []string{"huggingface-cli", "hf"},
		warning: "Stop a download before the move. Exodrive cannot see every tool that reads these models.",
	},
	{
		tool:    "lmstudio",
		id:      "lmstudio.models",
		title:   "LM Studio Models",
		deflt:   filepath.Join(".lmstudio", "models"),
		subdir:  filepath.Join("lmstudio", "models"),
		lock:    []string{"lms"},
		warning: "Quit LM Studio before the move. Exodrive only finds the lms command.",
	},
}

// ModelProvider moves one AI model store. All three stores run the same
// sequence, so one implementation serves every entry in modelStores.
type ModelProvider struct {
	store modelStore
}

var _ Provider = (*ModelProvider)(nil)

// Models returns one provider for each AI model store.
func Models() []Provider {
	providers := make([]Provider, 0, len(modelStores))
	for _, store := range modelStores {
		providers = append(providers, &ModelProvider{store: store})
	}
	return providers
}

func (p *ModelProvider) Name() string { return p.store.tool }

// root returns the folder that the tool reads its models from.
func (p *ModelProvider) root() string {
	if p.store.env != "" {
		if root := strings.TrimSpace(os.Getenv(p.store.env)); root != "" {
			return filepath.Join(root, p.store.envSub)
		}
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, p.store.deflt)
}

func (p *ModelProvider) Detect(ctx context.Context) ([]core.CacheItem, error) {
	path := p.root()
	if path == "" {
		return nil, nil
	}
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		return nil, nil
	}

	size, _ := fsutil.DirSize(ctx, path)
	return []core.CacheItem{{
		ID:             p.store.id,
		ToolName:       p.store.tool,
		Title:          p.store.title,
		Category:       core.CategoryModelStore,
		CurrentPath:    path,
		ConfiguredPath: filepath.Dir(path),
		SizeBytes:      size,
		IsActive:       true,
		IsMigrated:     movedOffVolume(path),
		IsSymlink:      fsutil.IsSymlink(path),
		RedirectMethod: core.RedirectSymlink,
		TargetSubdir:   p.store.subdir,
		Warning:        p.store.warning,
	}}, nil
}

func (p *ModelProvider) PreflightCheck(ctx context.Context, item core.CacheItem, targetRoot string) error {
	running, err := fsutil.CheckRunningProcesses(p.store.lock...)
	if err != nil {
		return err
	}
	if len(running) > 0 {
		return fmt.Errorf("%s is running (PID: %s): stop it before migration", running[0].Name, running[0].PID)
	}
	return nil
}

func (p *ModelProvider) Migrate(ctx context.Context, item core.CacheItem, targetRoot string, progress fsutil.MigrateProgress) error {
	dest := filepath.Join(targetRoot, item.TargetSubdir)
	if err := os.MkdirAll(dest, 0755); err != nil {
		return err
	}

	if item.IsSymlink {
		return fmt.Errorf("%s is already a symlink: nothing to migrate", item.CurrentPath)
	}

	if err := fsutil.CopyDir(item.CurrentPath, dest, item.SizeBytes, progress.Copy); err != nil {
		return fmt.Errorf("copy %s: %w", item.Title, err)
	}
	if err := fsutil.VerifyTreeProgress(item.CurrentPath, dest, progress.Verify); err != nil {
		return fmt.Errorf("%s: %w", item.Title, err)
	}

	if err := core.RecordMigration(core.MigrationRecord{
		ID:             item.ID,
		ToolName:       p.Name(),
		OriginalPath:   item.CurrentPath,
		DestinationDir: dest,
		RedirectMethod: core.RedirectSymlink,
		OriginalConfig: item.ConfiguredPath,
		MigratedAt:     time.Now(),
		SizeBytes:      item.SizeBytes,
	}); err != nil {
		return err
	}

	backupDir := item.CurrentPath + ".bak"
	if err := fsutil.ReplaceDirWithSymlink(item.CurrentPath, dest, backupDir); err != nil {
		return switchFailed(item.ID, err)
	}

	return discardLocalBackup(item.CurrentPath)
}

func (p *ModelProvider) Rollback(ctx context.Context, item core.CacheItem) error {
	rec, err := loadRecord(item.ID)
	if err != nil {
		return err
	}

	if err := restoreSymlink(rec.OriginalPath, rec.DestinationDir); err != nil {
		return err
	}

	return core.RemoveRecord(item.ID)
}
