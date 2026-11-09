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
			if !strings.Contains(out, "Exodrive v") {
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
