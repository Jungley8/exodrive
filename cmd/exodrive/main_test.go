package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"exodrive/internal/core"
	"exodrive/internal/fsutil"
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

// A command that stops writes one line, and doctor prints the line. A person
// pastes the output into an issue.
func TestCLI_RecordsAFailureAndDoctorPrintsIt(t *testing.T) {
	home := t.TempDir()
	env := map[string]string{
		"HOME":                    home,
		"EXODRIVE_MANIFEST_PATH":  filepath.Join(home, "manifest.json"),
		"EXODRIVE_NO_FAILURE_LOG": "",
	}

	out, code := runCLIEnv(t, env, "rollback", "npm.cache")
	if code != 1 {
		t.Fatalf("rollback of a missing record exits %d, want 1; output: %s", code, out)
	}

	logPath := filepath.Join(home, "failures.jsonl")
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("the command wrote no failure log: %v", err)
	}
	if !strings.Contains(string(data), `"action":"rollback"`) || !strings.Contains(string(data), `"item":"npm.cache"`) {
		t.Fatalf("the record does not name the command: %s", data)
	}
	if strings.Contains(string(data), home) {
		t.Fatalf("the record holds the home folder: %s", data)
	}

	out, code = runCLIEnv(t, env, "doctor")
	if code != 0 {
		t.Fatalf("doctor exits %d, want 0; output: %s", code, out)
	}
	for _, want := range []string{"Exodrive dev (", "Ledger:", "Failure log:", "rollback", "npm.cache"} {
		if !strings.Contains(out, want) {
			t.Fatalf("doctor does not print %q:\n%s", want, out)
		}
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

// testEnv merges the test overrides into the environment of this process, so a
// test never reads the developer's caches.
func testEnv(overrides map[string]string) []string {
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
	return env
}

func runCLIEnv(t *testing.T, overrides map[string]string, args ...string) (string, int) {
	t.Helper()
	cmd := exec.Command(testBinary, args...)
	cmd.Env = testEnv(overrides)

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

// runCLIStdout keeps stdout apart from stderr, so a test can prove that --json
// writes JSON and nothing else to stdout.
func runCLIStdout(t *testing.T, overrides map[string]string, args ...string) (string, string, int) {
	t.Helper()
	cmd := exec.Command(testBinary, args...)
	cmd.Env = testEnv(overrides)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	exitCode := 0
	if err := cmd.Run(); err != nil {
		exitErr, ok := err.(*exec.ExitError)
		if !ok {
			t.Fatalf("command failed with non-exit error: %v", err)
		}
		exitCode = exitErr.ExitCode()
	}
	return stdout.String(), stderr.String(), exitCode
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

// TestCLI_RemindsAboutAFolderOnThisMac keeps the reminder a reminder: the run
// says that a folder on this Mac frees no space, and it still plans the move.
func TestCLI_RemindsAboutAFolderOnThisMac(t *testing.T) {
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
		t.Fatalf("the reminder stopped the dry run (exit %d):\n%s", code, out)
	}
	if !strings.Contains(out, "Warning: the target folder is on this computer. A move here frees no space.") {
		t.Fatalf("the run does not warn about a folder on this Mac:\n%s", out)
	}
	if !strings.Contains(out, "Dry run.") {
		t.Fatalf("the warning replaced the plan:\n%s", out)
	}
}

// verifyFixture writes a ledger that records one moved copy, and returns the
// environment that points the command at it.
type verifyFixture struct {
	env  map[string]string
	copy string
}

func newVerifyFixture(t *testing.T, withDigest bool) verifyFixture {
	t.Helper()
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	copyDir := filepath.Join(tmp, "external", "npm-cache")
	for _, dir := range []string{home, copyDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(copyDir, "sample.tgz"), []byte("npm-tarball"), 0644); err != nil {
		t.Fatal(err)
	}

	summary, err := fsutil.DigestPath(copyDir)
	if err != nil {
		t.Fatalf("DigestPath failed: %v", err)
	}

	rec := core.MigrationRecord{
		ID:             "npm.cache",
		ToolName:       "npm",
		OriginalPath:   filepath.Join(home, ".npm"),
		DestinationDir: copyDir,
		RedirectMethod: core.RedirectNativeConfig,
		MigratedAt:     time.Now(),
		SizeBytes:      summary.Bytes,
	}
	if withDigest {
		rec.TreeDigest = string(summary.Digest)
		rec.FileCount = summary.Files
		rec.TotalBytes = summary.Bytes
	}

	ledger := filepath.Join(home, "manifest.json")
	data, err := json.MarshalIndent(core.Manifest{
		Version: core.ManifestVersion,
		Records: map[string]core.MigrationRecord{rec.ID: rec},
	}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ledger, data, 0644); err != nil {
		t.Fatal(err)
	}

	return verifyFixture{
		env: map[string]string{
			"HOME":                    home,
			"EXODRIVE_MANIFEST_PATH":  ledger,
			"EXODRIVE_NO_FAILURE_LOG": "",
		},
		copy: copyDir,
	}
}

func TestCLI_VerifyAcceptsAGoodCopy(t *testing.T) {
	f := newVerifyFixture(t, true)

	out, code := runCLIEnv(t, f.env, "verify", "npm.cache")
	if code != 0 {
		t.Fatalf("verify of a good copy exits %d, want 0:\n%s", code, out)
	}
	if !strings.Contains(out, "[ok] npm.cache") {
		t.Fatalf("verify does not report the state:\n%s", out)
	}
	if !strings.Contains(out, "Every copy matches its move.") {
		t.Fatalf("verify does not report the summary:\n%s", out)
	}
}

func TestCLI_VerifyReportsADamagedCopy(t *testing.T) {
	f := newVerifyFixture(t, true)
	if err := os.WriteFile(filepath.Join(f.copy, "sample.tgz"), []byte("npm-tarbal"), 0644); err != nil {
		t.Fatal(err)
	}

	out, code := runCLIEnv(t, f.env, "verify", "npm.cache")
	if code != 1 {
		t.Fatalf("verify of a damaged copy exits %d, want 1:\n%s", code, out)
	}
	if !strings.Contains(out, "[changed] npm.cache") {
		t.Fatalf("verify does not report changed:\n%s", out)
	}
	if !strings.Contains(out, "1 copy needs attention") {
		t.Fatalf("verify does not report the summary:\n%s", out)
	}
}

func TestCLI_VerifyReportsAMissingCopy(t *testing.T) {
	f := newVerifyFixture(t, true)
	if err := os.RemoveAll(f.copy); err != nil {
		t.Fatal(err)
	}

	out, code := runCLIEnv(t, f.env, "verify", "npm.cache")
	if code != 1 {
		t.Fatalf("verify of a missing copy exits %d, want 1:\n%s", code, out)
	}
	if !strings.Contains(out, "[missing] npm.cache") || !strings.Contains(out, "Connect the drive") {
		t.Fatalf("verify does not report the missing copy with a next step:\n%s", out)
	}
}

// TestCLI_VerifyNeverCallsAnOldRecordOK keeps the answer honest for a record
// that predates the digest: the tool cannot prove a copy it never measured.
func TestCLI_VerifyNeverCallsAnOldRecordOK(t *testing.T) {
	f := newVerifyFixture(t, false)

	out, code := runCLIEnv(t, f.env, "verify", "npm.cache")
	if code != 1 {
		t.Fatalf("verify of a record without a digest exits %d, want 1:\n%s", code, out)
	}
	if !strings.Contains(out, "[unverified] npm.cache") {
		t.Fatalf("verify does not report unverified:\n%s", out)
	}
}

func TestCLI_VerifyAllJSONWritesJSONToStdout(t *testing.T) {
	f := newVerifyFixture(t, true)

	stdout, stderr, code := runCLIStdout(t, f.env, "verify", "--all", "--json")
	if code != 0 {
		t.Fatalf("verify --all exits %d, want 0; stderr: %s", code, stderr)
	}
	if stderr != "" {
		t.Fatalf("verify --all wrote to stderr: %s", stderr)
	}

	var results []core.VerifyResult
	if err := json.Unmarshal([]byte(stdout), &results); err != nil {
		t.Fatalf("stdout is not JSON: %v\n%s", err, stdout)
	}
	if len(results) != 1 || results[0].ID != "npm.cache" || results[0].State != core.VerifyOK {
		t.Fatalf("results are %+v, want one ok npm copy", results)
	}
}

func TestCLI_VerifyUsage(t *testing.T) {
	f := newVerifyFixture(t, true)

	out, code := runCLIEnv(t, f.env, "verify")
	if code != 1 || !strings.Contains(out, "Usage: exodrive verify") {
		t.Fatalf("verify without an item exits %d:\n%s", code, out)
	}

	out, code = runCLIEnv(t, f.env, "verify", "unknown.item")
	if code != 1 || !strings.Contains(out, "holds no move for 'unknown.item'") {
		t.Fatalf("verify of an unknown item exits %d:\n%s", code, out)
	}
}
