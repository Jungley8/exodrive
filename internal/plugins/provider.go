package plugins

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"

	"exodrive/internal/core"
	"exodrive/internal/fsutil"
)

// Provider abstracts inspection and migration for a specific developer toolchain.
type Provider interface {
	Name() string
	Detect(ctx context.Context) ([]core.CacheItem, error)
	PreflightCheck(ctx context.Context, item core.CacheItem, targetRoot string) error
	Migrate(ctx context.Context, item core.CacheItem, targetRoot string, progress fsutil.MigrateProgress) error
	Rollback(ctx context.Context, item core.CacheItem) error
}

var (
	_ Provider = (*NPMProvider)(nil)
	_ Provider = (*PNPMProvider)(nil)
	_ Provider = (*YarnProvider)(nil)
	_ Provider = (*CargoProvider)(nil)
	_ Provider = (*GoProvider)(nil)
	_ Provider = (*BunProvider)(nil)
	_ Provider = (*UVProvider)(nil)
	_ Provider = (*PipProvider)(nil)
	_ Provider = (*DockerProvider)(nil)
	_ Provider = (*OrphanProvider)(nil)
)

// Registry maintains the active set of registered providers.
type Registry struct {
	providers []Provider
}

// NewRegistry initializes the registry with all standard toolchain providers.
func NewRegistry() *Registry {
	r := &Registry{}
	r.Register(
		NewNPMProvider(),
		NewPNPMProvider(),
		NewYarnProvider(),
		NewCargoProvider(),
		NewGoProvider(),
		NewBunProvider(),
		NewUVProvider(),
		NewPipProvider(),
		NewDockerProvider(),
		NewXcodeProvider(),
		NewMavenProvider(),
		NewGradleProvider(),
		NewCocoaPodsProvider(),
		NewSwiftPMProvider(),
		NewDartProvider(),
		NewAndroidProvider(),
		NewCondaProvider(),
		NewHomebrewProvider(),
		NewRubyProvider(),
		NewComposerProvider(),
		NewNuGetProvider(),
		NewDevtoolsProvider(),
		NewPlaywrightProvider(),
		NewCypressProvider(),
		NewPuppeteerProvider(),
		NewOrphanProvider(),
	)
	r.Register(Models()...)
	return r
}

func (r *Registry) Register(p ...Provider) {
	r.providers = append(r.providers, p...)
}

func (r *Registry) All() []Provider {
	return r.providers
}

func (r *Registry) Find(name string) Provider {
	for _, p := range r.providers {
		if p.Name() == name {
			return p
		}
	}
	return nil
}

// toolVersion returns the version that a tool reports, empty when the tool does
// not answer. A scan shows it, so a user can tell which layout Exodrive sees.
func toolVersion(ctx context.Context, tool string) string {
	out, err := exec.CommandContext(ctx, tool, "--version").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// toolQuery asks a tool for a path that only the tool knows. It reports whether
// the tool answered, so a missing binary never turns into a guessed path.
func toolQuery(ctx context.Context, name string, args ...string) (string, bool) {
	cmd := exec.CommandContext(ctx, name, args...)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = io.Discard
	if err := cmd.Run(); err != nil {
		return "", false
	}
	path := strings.TrimSpace(out.String())
	if path == "" {
		return "", false
	}
	return path, true
}

// toolMissingReason tells the user how to make the tool answer. Exodrive refuses
// a cache when it cannot confirm which folder the tool reads.
func toolMissingReason(tool, command string) string {
	return fmt.Sprintf("%s did not answer '%s'. Put %s on PATH, then scan again.", tool, command, tool)
}

// movedOffVolume reports whether a cache path already points away from the
// volume that holds the home folder. The ledger holds every move Exodrive made,
// so this covers a path that the user or another tool moved by hand.
func movedOffVolume(path string) bool {
	return fsutil.IsSymlink(path) || fsutil.IsOnExternalVolume(path)
}
