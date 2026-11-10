package plugins

import (
	"context"
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
	Migrate(ctx context.Context, item core.CacheItem, targetRoot string, onProgress fsutil.CopyProgressCallback) error
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
		NewOrphanProvider(),
	)
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
