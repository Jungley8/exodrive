package desktop

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/oneo-me/nsgo/nsgotest"
)

func TestNewerVersionReadsAFullVersion(t *testing.T) {
	cases := []struct {
		current, tag string
		want         bool
	}{
		{"0.6.0", "v0.6.1", true},
		{"0.6.0", "v0.7.0", true},
		{"0.6.0", "v1.0.0", true},
		{"0.6.0", "v0.6.0", false},
		{"0.6.0", "v0.5.9", false},
		{"0.6.0", "v0.6.0-beta", false},
		{"dev", "v9.9.9", false},
		{"0.6.0", "latest", false},
		{"0.6.0", "", false},
		{"v0.6", "v0.7", true},
	}
	for _, tc := range cases {
		if got := newerVersion(tc.current, tc.tag); got != tc.want {
			t.Errorf("newerVersion(%q, %q) is %v, want %v", tc.current, tc.tag, got, tc.want)
		}
	}
}

func TestLatestReleaseReadsTheAnswer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Accept"); !strings.Contains(got, "github") {
			t.Errorf("the request asks for %q", got)
		}
		_, _ = w.Write([]byte(`{"tag_name":"v0.7.0","html_url":"https://example.com/v0.7.0"}`))
	}))
	t.Cleanup(srv.Close)

	tag, url, err := latestRelease(t.Context(), srv.URL)
	if err != nil || tag != "v0.7.0" || url != "https://example.com/v0.7.0" {
		t.Fatalf("the answer reads %q, %q, %v", tag, url, err)
	}
}

func TestLatestReleaseRefusesAnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	t.Cleanup(srv.Close)

	if _, _, err := latestRelease(t.Context(), srv.URL); err == nil {
		t.Fatal("a refused request read as a release")
	}
}

// The check names a newer release and opens its page. A build without a version
// asks nothing.
func TestWindowOffersANewerRelease(t *testing.T) {
	ledgerPath(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"tag_name":"v9.9.9","html_url":"https://example.com/v9.9.9"}`))
	}))
	t.Cleanup(srv.Close)

	previousURL, previousVersion := latestReleaseURL, Version
	latestReleaseURL, Version = srv.URL, "0.6.0"
	t.Cleanup(func() { latestReleaseURL, Version = previousURL, previousVersion })

	app := nsgotest.Start(t, NewMain(newFakeEngine(fakeCache())), nsgotest.Size(1100, 800))
	app.WaitUntil(3*time.Second, func() bool {
		return strings.Contains(app.Dump(), "Version v9.9.9 is available.")
	})
	app.Press("id:update-open")
	app.Settle()

	if len(app.OpenedURLs) != 1 || app.OpenedURLs[0] != "https://example.com/v9.9.9" {
		t.Fatalf("the window opened %v", app.OpenedURLs)
	}
}

// A build with no version, and a machine that turned the check off, ask nothing.
func TestUpdateCheckStaysQuiet(t *testing.T) {
	ledgerPath(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"tag_name":"v9.9.9","html_url":"https://example.com/v9.9.9"}`))
	}))
	t.Cleanup(srv.Close)

	previousURL := latestReleaseURL
	latestReleaseURL = srv.URL
	t.Cleanup(func() { latestReleaseURL = previousURL })

	t.Setenv(NoUpdateCheckEnvVar, "1")
	app := nsgotest.Start(t, NewMain(newFakeEngine(fakeCache())), nsgotest.Size(1100, 800))
	app.WaitUntil(2*time.Second, func() bool { return app.Exists("text:Fake Cache") })
	time.Sleep(100 * time.Millisecond)
	if app.Exists("id:update-open") {
		t.Fatalf("the check ran with %s set:\n%s", NoUpdateCheckEnvVar, app.Dump())
	}
}
