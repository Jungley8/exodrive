package plugins

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"exodrive/internal/core"
)

// playwrightBrowsers returns the folder where Playwright keeps the browsers it
// downloads. PLAYWRIGHT_BROWSERS_PATH names it. The value 0 asks Playwright to
// keep the browsers inside node_modules, so it does not name a folder.
func playwrightBrowsers() string {
	if dir := strings.TrimSpace(os.Getenv("PLAYWRIGHT_BROWSERS_PATH")); dir != "" && dir != "0" {
		return dir
	}
	return osCacheDir("ms-playwright")
}

// cypressCache returns the folder where Cypress keeps the binary bundle it
// downloads. CYPRESS_CACHE_FOLDER names it.
func cypressCache() string {
	if dir := strings.TrimSpace(os.Getenv("CYPRESS_CACHE_FOLDER")); dir != "" {
		return dir
	}
	if runtime.GOOS == "windows" {
		return under(strings.TrimSpace(os.Getenv("LOCALAPPDATA")), filepath.Join("Cypress", "Cache"))
	}
	return osCacheDir("Cypress")
}

// puppeteerBrowsers returns the folder where Puppeteer keeps the browsers it
// downloads. PUPPETEER_CACHE_DIR names it, and ~/.cache/puppeteer is the
// default on every platform.
func puppeteerBrowsers() string {
	if dir := strings.TrimSpace(os.Getenv("PUPPETEER_CACHE_DIR")); dir != "" {
		return dir
	}
	return homePath(".cache", "puppeteer")
}

// browserFolders move the stores that browser test tools fill. Each tool
// downloads the files again when it needs them, so a wrong move costs a
// download and no data.
//
// No process lock names these tools. The run that reads the store is `node`,
// and the browser program names are shared with programs that never touch this
// folder. A name lock would either miss the writer or stop an unrelated
// program, so the warning carries the next step instead.
var browserFolders = []linkedRow{
	{
		id:       "playwright.browsers",
		title:    "Playwright Browsers",
		path:     playwrightBrowsers,
		category: core.CategoryPackageStore,
		subdir:   filepath.Join("playwright", "browsers"),
		warning:  "Stop every test run that uses Playwright before the move. Playwright downloads a browser again when it needs one.",
	},
	{
		id:       "cypress.binaries",
		title:    "Cypress Binaries",
		path:     cypressCache,
		category: core.CategoryPackageStore,
		subdir:   filepath.Join("cypress", "binaries"),
		warning:  "Stop every test run that uses Cypress before the move. Cypress downloads its binary again when it needs it.",
	},
	{
		id:       "puppeteer.browsers",
		title:    "Puppeteer Browsers",
		path:     puppeteerBrowsers,
		category: core.CategoryPackageStore,
		subdir:   filepath.Join("puppeteer", "browsers"),
		warning:  "Stop every test run that uses Puppeteer before the move. Puppeteer downloads a browser again when it needs one.",
	},
}

// NewPlaywrightProvider moves the browsers that Playwright downloaded.
func NewPlaywrightProvider() *linkedProvider {
	return &linkedProvider{tool: "playwright", rows: []linkedRow{browserFolders[0]}}
}

// NewCypressProvider moves the binary bundle that Cypress downloaded.
func NewCypressProvider() *linkedProvider {
	return &linkedProvider{tool: "cypress", rows: []linkedRow{browserFolders[1]}}
}

// NewPuppeteerProvider moves the browsers that Puppeteer downloaded.
func NewPuppeteerProvider() *linkedProvider {
	return &linkedProvider{tool: "puppeteer", rows: []linkedRow{browserFolders[2]}}
}
