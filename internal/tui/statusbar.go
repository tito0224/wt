package tui

import (
	"fmt"
	"strings"

	"github.com/johnlotito/wt/internal/forge"
)

// keyHint is a single key/label pair shown in the footer key bar.
type keyHint struct {
	key   string
	label string
}

// listKeyHints are the normal-mode key hints, in display order.
var listKeyHints = []keyHint{
	{"↵", "cd"},
	{"a", "add"},
	{"o", "open"},
	{"⇥", "actions"},
	{"r", "remove"},
	{"/", "search"},
	{"q", "quit"},
}

// renderKeyBar renders key hints as reverse-video "caps" followed by dim
// labels, separated by wide gaps. The bright caps give the eye a fixed anchor
// per action, which a flat "key verb · key verb" line lacks.
func renderKeyBar(hints []keyHint) string {
	parts := make([]string, len(hints))
	for i, h := range hints {
		cap := keycapStyle.Render(" " + h.key + " ")
		parts[i] = cap + " " + labelStyle.Render(h.label)
	}
	return strings.Join(parts, "   ")
}

// statusBar renders the two-line detail panel for the selected worktree: a
// commit line and a PR line. Content loads asynchronously, so it shows a
// muted placeholder until the detail arrives.
func (m *model) statusBar() string {
	wt := m.selected()
	if wt == nil {
		return ""
	}

	d, loaded := m.details[wt.Path]

	var commitLine string
	switch {
	case !loaded:
		commitLine = pathStyle.Render("  loading…")
	case d.commitErr != nil:
		commitLine = pathStyle.Render("  (no commits yet)")
	default:
		c := d.commit
		meta := currentStyle.Render(c.ShortHash) + pathStyle.Render(
			fmt.Sprintf("  %s · %s", c.RelDate, c.Author))
		commitLine = "  " + meta + "\n  " + headerStyle.Render(trunc(c.Subject, max(1, m.width-2)))
	}

	if pr := m.prLine(d, loaded); pr != "" {
		return commitLine + "\n" + pr
	}
	return commitLine
}

// prLine renders the pull-request line of the status bar.
func (m *model) prLine(d detail, loaded bool) string {
	if !loaded {
		return ""
	}
	if !d.prChecked || d.pr == nil {
		// Either no forge/PR, or a definitive "no PR". Keep it quiet.
		return ""
	}
	return "  " + prBadge(d.pr) + " " + pathStyle.Render(d.pr.URL)
}

// prBadge renders a colored [#123 state] badge for a PR.
func prBadge(pr *forge.PR) string {
	state := pr.State
	if pr.IsDraft && state == "OPEN" {
		state = "DRAFT"
	}
	var st = pathStyle
	switch state {
	case "OPEN":
		st = prOpenStyle
	case "MERGED":
		st = prMergedSty
	case "CLOSED":
		st = prClosedSty
	}
	return st.Render(fmt.Sprintf("#%d %s", pr.Number, strings.ToLower(state)))
}
