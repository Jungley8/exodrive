package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

var testBinary string

func TestMain(m *testing.M) {
	tmpDir, err := os.MkdirTemp("", "exodrive-cli-test-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed creating temp dir: %v\n", err)
		os.Exit(1)
	}
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	binPath := filepath.Join(tmpDir, "exodrive")
	cmd := exec.Command("go", "build", "-o", binPath, ".")
	if out, err := cmd.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "failed building exodrive test binary: %v\n%s\n", err, string(out))
		os.Exit(1)
	}

	testBinary = binPath
	os.Exit(m.Run())
}

func runCLI(t *testing.T, env []string, args ...string) (string, int) {
	t.Helper()
	cmd := exec.Command(testBinary, args...)
	if len(env) > 0 {
		cmd.Env = append(os.Environ(), env...)
	}
	out, err := cmd.CombinedOutput()
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			t.Fatalf("command failed with non-exit error: %v", err)
		}
	}
	return string(out), exitCode
}

func TestCLI_Version(t *testing.T) {
	for _, flag := range []string{"version", "--version", "-v"} {
		t.Run(flag, func(t *testing.T) {
			out, code := runCLI(t, nil, flag)
			if code != 0 {
				t.Errorf("expected exit code 0, got %d", code)
			}
			// The test build carries no ldflags, so the version is "dev".
			if !strings.Contains(out, "Exodrive dev (") {
				t.Errorf("expected version output, got: %s", out)
			}
		})
	}
}

func TestCLI_Help(t *testing.T) {
	for _, flag := range []string{"help", "--help", "-h"} {
		t.Run(flag, func(t *testing.T) {
			out, code := runCLI(t, nil, flag)
			if code != 0 {
				t.Errorf("expected exit code 0, got %d", code)
			}
			if !strings.Contains(out, "Exodrive — Developer Cache Migrator") {
				t.Errorf("expected help output, got: %s", out)
			}
			if !strings.Contains(out, "preflight") {
				t.Errorf("expected preflight in help options, got: %s", out)
			}
		})
	}
}

func TestCLI_UnknownCommand(t *testing.T) {
	out, code := runCLI(t, nil, "unknown_command_xyz")
	if code != 1 {
		t.Errorf("expected exit code 1 on unknown command, got %d", code)
	}
	if !strings.Contains(out, "Unknown command 'unknown_command_xyz'") {
		t.Errorf("unexpected output: %s", out)
	}
}

func TestCLI_Rollback_ExitCodes(t *testing.T) {
	t.Run("missing args", func(t *testing.T) {
		out, code := runCLI(t, nil, "rollback")
		if code != 1 {
			t.Errorf("expected exit code 1 on missing args, got %d", code)
		}
		if !strings.Contains(out, "Usage: exodrive rollback") {
			t.Errorf("unexpected output: %s", out)
		}
	})

	t.Run("nonexistent item", func(t *testing.T) {
		tempLedger := filepath.Join(t.TempDir(), "manifest.json")
		env := []string{"EXODRIVE_MANIFEST_PATH=" + tempLedger}
		out, code := runCLI(t, env, "rollback", "nonexistent.item")
		if code != 1 {
			t.Errorf("expected exit code 1 on nonexistent item rollback, got %d", code)
		}
		if !strings.Contains(out, "Rollback failed") {
			t.Errorf("unexpected output: %s", out)
		}
	})
}

func TestCLI_Preflight_ExitCodes(t *testing.T) {
	t.Run("missing item argument", func(t *testing.T) {
		out, code := runCLI(t, nil, "preflight")
		if code != 1 {
			t.Errorf("expected exit code 1, got %d", code)
		}
		if !strings.Contains(out, "Usage: exodrive preflight") {
			t.Errorf("unexpected output: %s", out)
		}
	})

	t.Run("missing target argument", func(t *testing.T) {
		out, code := runCLI(t, nil, "preflight", "npm.cache")
		if code != 1 {
			t.Errorf("expected exit code 1, got %d", code)
		}
		if !strings.Contains(out, "--target is required") {
			t.Errorf("unexpected output: %s", out)
		}
	})

	t.Run("nonexistent item", func(t *testing.T) {
		out, code := runCLI(t, nil, "preflight", "nonexistent.cache", "--target=/tmp/some-target")
		if code != 1 {
			t.Errorf("expected exit code 1, got %d", code)
		}
		if !strings.Contains(out, "not found") {
			t.Errorf("unexpected output: %s", out)
		}
	})
}

func TestCLI_Check_JSON(t *testing.T) {
	tempHome := t.TempDir()
	tempLedger := filepath.Join(tempHome, "manifest.json")
	env := []string{
		"HOME=" + tempHome,
		"EXODRIVE_MANIFEST_PATH=" + tempLedger,
	}

	out, code := runCLI(t, env, "check", "--json")
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d; output: %s", code, out)
	}

	var items []map[string]interface{}
	if err := json.Unmarshal([]byte(out), &items); err != nil {
		t.Fatalf("failed unmarshaling JSON output: %v\nOutput was: %s", err, out)
	}
}

func TestVersionLabel(t *testing.T) {
	cases := []struct {
		version string
		want    string
	}{
		{"0.1.0", "v0.1.0"},
		{"1.2.3", "v1.2.3"},
		{"v0.1.0", "v0.1.0"},
		{"dev", "dev"},
		{"fdc1fdd-dirty", "fdc1fdd-dirty"},
		{"", ""},
	}

	for _, tc := range cases {
		if got := versionLabel(tc.version); got != tc.want {
			t.Errorf("versionLabel(%q) = %q; want %q", tc.version, got, tc.want)
		}
	}
}

// runCLIEnv runs the CLI with the given variables replacing any inherited
// value, so a test never reads the developer's caches.
func runCLIEnv(t *testing.T, overrides map[string]string, args ...string) (string, int) {
	t.Helper()
	cmd := exec.Command(testBinary, args...)

	env := make([]string, 0, len(os.Environ())+len(overrides))
	for _, entry := range os.Environ() {
		key := strings.SplitN(entry, "=", 2)[0]
		if _, replaced := overrides[key]; replaced {
			continue
		}
		env = append(env, entry)
	}
	for key, value := range overrides {
		env = append(env, key+"="+value)
	}
	cmd.Env = env

	out, err := cmd.CombinedOutput()
	exitCode := 0
	if err != nil {
		exitErr, ok := err.(*exec.ExitError)
		if !ok {
			t.Fatalf("command failed with non-exit error: %v", err)
		}
		exitCode = exitErr.ExitCode()
	}
	return string(out), exitCode
}

// TestCLI_MigrateDryRunMovesNothing pins the rule that --dry-run reports the
// plan and changes nothing: no copy, no link, no ledger record.
func TestCLI_MigrateDryRunMovesNothing(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	cargoHome := filepath.Join(tmp, "cargo")
	registry := filepath.Join(cargoHome, "registry")
	target := filepath.Join(tmp, "target")
	ledger := filepath.Join(tmp, "manifest.json")

	for _, dir := range []string{home, registry, target} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(registry, "crate"), []byte("crate-bytes"), 0644); err != nil {
		t.Fatal(err)
	}

	env := map[string]string{
		"HOME":                   home,
		"CARGO_HOME":             cargoHome,
		"EXODRIVE_MANIFEST_PATH": ledger,
		"GOCACHE":                filepath.Join(home, "gocache"),
		"GOMODCACHE":             filepath.Join(home, "gomodcache"),
		"UV_CACHE_DIR":           filepath.Join(home, "uv"),
		"PIP_CACHE_DIR":          filepath.Join(home, "pip"),
		"BUN_INSTALL":            filepath.Join(home, "bun"),
		"YARN_CACHE_FOLDER":      filepath.Join(home, "yarn"),
		"ORBSTACK_DATA_DIR":      filepath.Join(home, "orbstack"),
	}

	out, code := runCLIEnv(t, env, "migrate", "--dry-run", "--tools=cargo", "--target="+target)
	if code != 0 {
		t.Fatalf("dry run exited %d:\n%s", code, out)
	}
	if !strings.Contains(out, "Dry run.") {
		t.Fatalf("dry run did not report the plan:\n%s", out)
	}

	info, err := os.Lstat(registry)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		t.Fatal("dry run replaced the local cache with a symlink")
	}
	if _, err := os.Stat(filepath.Join(target, "cargo", "registry")); !os.IsNotExist(err) {
		t.Fatal("dry run copied data to the target")
	}
	if data, err := os.ReadFile(ledger); err == nil && strings.Contains(string(data), "cargo.registry") {
		t.Fatalf("dry run wrote a ledger record:\n%s", data)
	}
}

// TestCLI_ReportsBlockedCache pins the rule that a cache Exodrive cannot move
// stays visible in the plan, with its reason, and stops the run when nothing
// else is selected.
func TestCLI_ReportsBlockedCache(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	bin := filepath.Join(tmp, "bin")
	cacheDir := filepath.Join(home, ".yarn", "berry", "cache")
	target := filepath.Join(tmp, "target")
	ledger := filepath.Join(tmp, "manifest.json")

	for _, dir := range []string{bin, cacheDir, target} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(cacheDir, "pkg.zip"), []byte("berry"), 0644); err != nil {
		t.Fatal(err)
	}

	yarn := filepath.Join(bin, "yarn")
	script := "#!/bin/sh\n" +
		"case \"$1\" in\n" +
		"--version) echo 4.1.0 ;;\n" +
		"config) if [ \"$2\" = get ]; then echo \"$MOCK_YARN_CACHE\"; fi ;;\n" +
		"esac\n"
	if err := os.WriteFile(yarn, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}

	env := map[string]string{
		"HOME":                   home,
		"PATH":                   bin + string(os.PathListSeparator) + os.Getenv("PATH"),
		"MOCK_YARN_CACHE":        cacheDir,
		"YARN_CACHE_FOLDER":      "",
		"EXODRIVE_MANIFEST_PATH": ledger,
		"GOCACHE":                filepath.Join(home, "gocache"),
		"GOMODCACHE":             filepath.Join(home, "gomodcache"),
		"UV_CACHE_DIR":           filepath.Join(home, "uv"),
		"PIP_CACHE_DIR":          filepath.Join(home, "pip"),
		"BUN_INSTALL":            filepath.Join(home, "bun"),
		"CARGO_HOME":             filepath.Join(home, "cargo"),
		"ORBSTACK_DATA_DIR":      filepath.Join(home, "orbstack"),
	}

	out, code := runCLIEnv(t, env, "migrate", "--dry-run", "--tools=yarn", "--target="+target)
	if code != 1 {
		t.Fatalf("expected exit 1 when every selected cache is blocked, got %d:\n%s", code, out)
	}
	if !strings.Contains(out, "[blocked]") || !strings.Contains(out, "Yarn 4.1.0") {
		t.Fatalf("the plan does not report the blocked cache:\n%s", out)
	}
	if _, err := os.Stat(filepath.Join(home, ".yarn", "berry", "cache", "pkg.zip")); err != nil {
		t.Fatalf("the blocked cache changed: %v", err)
	}

	checkOut, _ := runCLIEnv(t, env, "check")
	if !strings.Contains(checkOut, "Blocked") {
		t.Fatalf("check does not show the Blocked status:\n%s", checkOut)
	}
}
