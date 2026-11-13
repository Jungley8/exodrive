package desktop

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/oneo-me/nsgo/core"
	"github.com/oneo-me/nsgo/icons/lucide"
	"github.com/oneo-me/nsgo/ui"
)

// Version is the build's version. The release build sets it with -ldflags.
var Version = "dev"

// ReleasePage is where a person reads the notes and downloads the build.
const ReleasePage = "https://github.com/Jungley8/exodrive/releases/latest"

// latestReleaseURL answers with the newest release. A test points it at its own
// server.
var latestReleaseURL = "https://api.github.com/repos/Jungley8/exodrive/releases/latest"

// NoUpdateCheckEnvVar turns the check off, for an offline machine or a script.
const NoUpdateCheckEnvVar = "EXODRIVE_NO_UPDATE_CHECK"

// updateState is the release that the check found, or an empty one.
type updateState struct {
	Tag string
	URL string
}

// checkForUpdate asks for the newest release and reports one that is newer than
// this build. It fails quiet: no network, no answer, or a version it cannot
// read all leave the window as it is.
func checkForUpdate(cx *core.Scope, state *AppState) {
	if Version == "" || Version == "dev" || os.Getenv(NoUpdateCheckEnvVar) != "" {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		tag, url, err := latestRelease(ctx, latestReleaseURL)
		if err != nil || !newerVersion(Version, tag) {
			return
		}
		if url == "" {
			url = ReleasePage
		}
		cx.Post(func() { state.Update.Set(updateState{Tag: tag, URL: url}) })
	}()
}

// latestRelease reads the newest release from the GitHub answer.
func latestRelease(ctx context.Context, endpoint string) (tag, url string, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("the release list answered %s", resp.Status)
	}
	var body struct {
		TagName string `json:"tag_name"`
		HTMLURL string `json:"html_url"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", "", err
	}
	return strings.TrimSpace(body.TagName), strings.TrimSpace(body.HTMLURL), nil
}

// newerVersion reports whether tag names a release newer than current. A name
// that is not a version compares as no release, so the window never offers a
// build it cannot name.
func newerVersion(current, tag string) bool {
	now, ok := parseVersion(current)
	if !ok {
		return false
	}
	next, ok := parseVersion(tag)
	if !ok {
		return false
	}
	for i := range now {
		if next[i] != now[i] {
			return next[i] > now[i]
		}
	}
	return false
}

// parseVersion reads a version such as v0.6.1 or 0.6. It reads the leading
// numbers of each part, so a suffix such as -beta does not hide the version.
func parseVersion(v string) ([3]int, bool) {
	var out [3]int
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	if v == "" {
		return out, false
	}
	parts := strings.SplitN(v, ".", 3)
	if len(parts) < 2 {
		return out, false
	}
	for i, part := range parts {
		digits := part
		if j := strings.IndexFunc(part, func(r rune) bool { return r < '0' || r > '9' }); j >= 0 {
			digits = part[:j]
		}
		n, err := strconv.Atoi(digits)
		if err != nil {
			return out, false
		}
		out[i] = n
	}
	return out, true
}

// updateLine names the newer release and opens its page.
func updateLine(cx *core.Scope, state *AppState) core.Node {
	colors := ui.ColorsOf(cx)
	update := state.Update.Get()
	if update.Tag == "" {
		return core.Group{}
	}
	url := update.URL
	return core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 8},
		Children: []core.Node{
			ui.Icon(lucide.Download, 14, colors.Accent),
			typeLine(fmt.Sprintf(state.Text.UpdateAvailable, update.Tag), typeSmall, colors.Text),
			ui.Button{Text: state.Text.UpdateOpen, Variant: ui.ButtonGhost, Size: ui.SizeSmall,
				OnPress: func() { cx.OpenURL(url, nil) }, TestID: "update-open"},
		}}
}
