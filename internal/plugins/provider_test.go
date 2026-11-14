package plugins

import (
	"context"
	"testing"
	"time"

	"exodrive/internal/core"
)

func TestRegistry(t *testing.T) {
	reg := NewRegistry()
	if reg == nil {
		t.Fatal("expected non-nil registry")
	}

	all := reg.All()
	if len(all) != 31 {
		t.Fatalf("expected 31 providers registered, got %d", len(all))
	}

	expectedNames := map[string]bool{
		"npm":           false,
		"pnpm":          false,
		"yarn":          false,
		"cargo":         false,
		"go":            false,
		"bun":           false,
		"uv":            false,
		"pip":           false,
		"docker":        false,
		"xcode":         false,
		"maven":         false,
		"gradle":        false,
		"cocoapods":     false,
		"swiftpm":       false,
		"dart":          false,
		"android":       false,
		"conda":         false,
		"homebrew":      false,
		"ruby":          false,
		"composer":      false,
		"nuget":         false,
		"devtools":      false,
		"playwright":    false,
		"cypress":       false,
		"puppeteer":     false,
		"golangci-lint": false,
		"gopls":         false,
		"orphan":        false,
		"ollama":        false,
		"huggingface":   false,
		"lmstudio":      false,
	}

	for _, p := range all {
		name := p.Name()
		if _, ok := expectedNames[name]; !ok {
			t.Errorf("unexpected provider name: %s", name)
		}
		expectedNames[name] = true

		found := reg.Find(name)
		if found == nil || found.Name() != name {
			t.Errorf("registry.Find(%q) failed or returned mismatched provider", name)
		}
	}

	for name, found := range expectedNames {
		if !found {
			t.Errorf("expected provider %q was not found in registry", name)
		}
	}

	if missing := reg.Find("nonexistent_tool"); missing != nil {
		t.Errorf("expected nil for nonexistent provider, got %v", missing)
	}
}

func TestProvidersDetectSmoke(t *testing.T) {
	reg := NewRegistry()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	for _, p := range reg.All() {
		t.Run(p.Name(), func(t *testing.T) {
			items, err := p.Detect(ctx)
			if err != nil {
				t.Logf("provider %s Detect returned error (expected in some CI environments): %v", p.Name(), err)
			}
			for _, item := range items {
				if item.ToolName == "" {
					t.Errorf("item missing ToolName: %+v", item)
				}
				if item.CurrentPath == "" {
					t.Errorf("item missing CurrentPath: %+v", item)
				}
			}
		})
	}
}

func TestProvidersPreflightCheck(t *testing.T) {
	reg := NewRegistry()
	ctx := context.Background()
	targetRoot := t.TempDir()

	for _, p := range reg.All() {
		t.Run(p.Name(), func(t *testing.T) {
			dummyItem := core.CacheItem{
				ID:          p.Name() + ".test",
				ToolName:    p.Name(),
				CurrentPath: t.TempDir(),
			}
			// PreflightCheck should not panic
			_ = p.PreflightCheck(ctx, dummyItem, targetRoot)
		})
	}
}
