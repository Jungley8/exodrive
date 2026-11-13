package desktop

import (
	"fmt"
	"net/url"
	"runtime"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/oneo-me/nsgo/core"

	exoCore "exodrive/internal/core"
)

// IssuesURL takes a report from the window. The window opens it in the
// browser; it never sends the report itself.
const IssuesURL = "https://github.com/Jungley8/exodrive/issues/new"

const (
	// reportRecords is how many records a report carries. The dialog shows
	// the same text that the link holds.
	reportRecords = 5
	// reportMessageLimit keeps one message short in a report.
	reportMessageLimit = 300
	// reportBodyLimit keeps the link a normal length, so every browser opens
	// it.
	reportBodyLimit = 2000
)

// shownFailure is a failure the window shows. The cause stays in memory only:
// the window needs its words again when a person copies the details.
type shownFailure struct {
	friendly
	Action string
	Item   string
	Cause  error
}

// recordFailure writes one record to the failure log. A person reads the same
// words on the card and in the report. The result of the action does not
// depend on the write: a full disk changes nothing.
func recordFailure(action, item string, err error) {
	if err == nil {
		return
	}
	_ = exoCore.RecordFailure(exoCore.Failure{
		Version: Version,
		Action:  action,
		Kind:    exoCore.FailureKind(err),
		Item:    item,
		Message: err.Error(),
	})
}

// openReport fills the report from the log, then shows the dialog. The text is
// read once, so a repaint never reads the log again.
func openReport(state *AppState) {
	state.ReportText.Set(reportText(state, exoCore.RecentFailures(reportRecords)))
	state.ReportOpen.Set(true)
}

// closeReport hides the dialog. The text stays, so a second press shows the
// same words.
func closeReport(state *AppState) { state.ReportOpen.Set(false) }

// recordOf turns a failure the window shows into a record for a report.
func recordOf(sf shownFailure) exoCore.Failure {
	f := exoCore.Failure{
		Version: Version,
		Action:  sf.Action,
		Item:    sf.Item,
	}
	if sf.Cause != nil {
		f.Kind = exoCore.FailureKind(sf.Cause)
		f.Message = sf.Cause.Error()
	}
	return f
}

// reportText writes the text that a person reads in the dialog, copies, and
// sends. Every message is redacted and cut, so the text holds no home folder
// and no long tool output.
func reportText(state *AppState, records []exoCore.Failure) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Exodrive %s (%s/%s), language %s", Version, runtime.GOOS, runtime.GOARCH, state.Lang)
	b.WriteString("\n\n")

	if len(records) == 0 {
		b.WriteString(state.Text.ReportEmpty)
		b.WriteString("\n")
	} else {
		for _, f := range records {
			b.WriteString(reportLine(f))
			b.WriteString("\n")
		}
		if len(records) == reportRecords {
			b.WriteString("\n")
			b.WriteString(state.Text.ReportCut)
			b.WriteString("\n")
		}
	}

	b.WriteString("\n")
	b.WriteString(state.Text.ReportFoot)
	return cutReport(b.String())
}

// reportLine names one record in one line. The time is cut to the minute, so
// the line stays short.
func reportLine(f exoCore.Failure) string {
	when := f.Time
	if len(when) >= 16 {
		when = when[:16]
	}
	item := ""
	if f.Item != "" {
		item = " " + f.Item
	}
	kind := f.Kind
	if kind == "" {
		kind = exoCore.KindOther
	}
	msg := exoCore.CutMessage(exoCore.Redact(f.Message))
	if len(msg) > reportMessageLimit {
		msg = cutUTF8(msg, reportMessageLimit)
	}
	if msg == "" {
		return fmt.Sprintf("%s %s %s%s", when, f.Action, kind, item)
	}
	return fmt.Sprintf("%s %s %s%s: %s", when, f.Action, kind, item, msg)
}

// cutReport keeps a report under the body limit on a line boundary, so the
// issue link opens everywhere.
func cutReport(s string) string {
	if len(s) <= reportBodyLimit {
		return s
	}
	head := cutUTF8(s, reportBodyLimit)
	if i := strings.LastIndexByte(head, '\n'); i > 0 {
		head = head[:i]
	}
	return head + "\n\n…"
}

// cutUTF8 cuts a string at n bytes and keeps the text valid.
func cutUTF8(s string, n int) string {
	if len(s) <= n {
		return s
	}
	b := []byte(s)[:n]
	for len(b) > 0 && !utf8.Valid(b) {
		b = b[:len(b)-1]
	}
	return string(b)
}

// reportURL builds the prefilled issue link. A press on the button opens it in
// the browser.
func reportURL(title, body string) string {
	q := url.Values{}
	q.Set("title", title)
	q.Set("body", body)
	return IssuesURL + "?" + q.Encode()
}

// reportTitle names the newest failure, so the issue list reads at a glance.
func reportTitle(records []exoCore.Failure, t text) string {
	if len(records) == 0 {
		return t.ReportTitleIssue
	}
	f := records[len(records)-1]
	kind := f.Kind
	if kind == "" {
		kind = exoCore.KindOther
	}
	title := fmt.Sprintf("Failure in %s: %s", f.Action, kind)
	if f.Item != "" {
		title += " (" + f.Item + ")"
	}
	return title
}

// copyReport puts the text on the clipboard and says that it did. A person can
// paste it anywhere, without an account.
func copyReport(cx *core.Scope, state *AppState, text string) {
	if text == "" {
		return
	}
	cx.SetClipboard(text)
	t := state.Text
	state.Notice.Set(notice{Kind: noticeInfo, Title: t.DetailsCopied, Body: t.DetailsCopiedBody})
}

// copyFailure copies the details of one cache failure, without opening the
// dialog. The record is the one the log holds, with its clock.
func copyFailure(cx *core.Scope, state *AppState, sf shownFailure) {
	rec := recordOf(sf)
	rec.Time = time.Now().UTC().Format(time.RFC3339)
	copyReport(cx, state, reportText(state, []exoCore.Failure{rec}))
}
