package tui

import (
	"fmt"
	"strings"

	"github.com/johnlotito/wt/internal/forge"
	"github.com/johnlotito/wt/internal/git"
)

// keyHint is a single key/label pair shown in the footer key bar. When
// enabled is false the hint is rendered dimmed, signalling the action applies
// in principle but not to the current selection (e.g. no PR, or clean tree).
type keyHint struct {
	key     string
	label   string
	enabled bool
}

// keyBarHints builds the compact key bar for a screen. Keys that are
// irrelevant to a screen are omitted entirely (keeping the bar short), while
// keys that apply but are unavailable for the current selection are included
// but dimmed (enabled=false). The full list is always available via `?`.
//
// hasPR toggles the open-PR key; dirty toggles the diff key.
func (m *model) keyBarHints(v view, hasPR, dirty bool) []keyHint {
	hints := []keyHint{{"↵", "cd", true}}
	if v == viewList {
		hints = append(hints, keyHint{"a", "add", true})
	}
	hints = append(hints,
		keyHint{"o", "open", true},
		keyHint{"p", "PR", hasPR},
		keyHint{"d", "diff", dirty},
	)
	if v == viewList {
		hints = append(hints, keyHint{"⇥", "details", true})
	} else {
		hints = append(hints, keyHint{"⇥", "back", true})
	}
	hints = append(hints, keyHint{"r", "remove", true})
	if v == viewList {
		hints = append(hints, keyHint{"/", "search", true})
	}
	hints = append(hints, keyHint{"?", "help", true}, keyHint{"q", "quit", true})
	return hints
}

// renderKeyBar renders key hints as reverse-video "caps" followed by dim
// labels, separated by wide gaps. The bright caps give the eye a fixed anchor
// per action, which a flat "key verb · key verb" line lacks. Disabled hints
// are fully dimmed so the layout stays constant across screens.
func renderKeyBar(hints []keyHint) string {
	parts := make([]string, len(hints))
	for i, h := range hints {
		if h.enabled {
			cap := keycapStyle.Render(" " + h.key + " ")
			parts[i] = cap + " " + labelStyle.Render(h.label)
		} else {
			// Dimmed, non-reverse cap so disabled actions recede.
			parts[i] = pathStyle.Render(" " + h.key + "  " + h.label)
		}
	}
	return strings.Join(parts, "   ")
}

// keyBarFooter renders the shared bottom-of-screen footer: a transient
// message or in-flight spinner line, then either a remove-confirm prompt or
// the given key bar. Used by both the list and detail screens so their footers
// behave identically.
func (m *model) keyBarFooter(hints []keyHint) string {
	var b strings.Builder

	// Confirmation prompts take over the footer when active.
	switch m.list.confirm {
	case "remove":
		wt := m.selected()
		name := ""
		if wt != nil {
			name = wt.Label()
		}
		warn := ""
		if wt != nil {
			warn = errorStyle.Render(m.removeWarning(wt))
		}
		b.WriteString(errorStyle.Render(fmt.Sprintf("remove %s?", name)) + warn +
			helpStyle.Render("  (") + keycapStyle.Render(" y ") + helpStyle.Render(" worktree  ") +
			keycapStyle.Render(" b ") + helpStyle.Render(" +branch  ") +
			keycapStyle.Render(" n ") + helpStyle.Render(" cancel)"))
		return b.String()
	case "force-branch":
		b.WriteString(errorStyle.Render(fmt.Sprintf("branch %s is unmerged — force-delete?", m.pendingBranch)) +
			helpStyle.Render("  (y/n)"))
		return b.String()
	}

	// Transient message or in-flight spinner line above the key bar.
	if m.spinner.active {
		b.WriteString(m.spinner.model.View() + " " +
			helpStyle.Render(m.spinner.msg) + "\n")
	} else if m.message != "" {
		if m.isErr {
			b.WriteString(errorStyle.Render("✗ "+m.message) + "\n")
		} else {
			b.WriteString(cleanStyle.Render("✓ "+m.message) + "\n")
		}
	}
	b.WriteString(renderKeyBar(hints))
	return b.String()
}

// removeWarning returns a bracketed warning about work that would be lost by
// removing wt: uncommitted changes, unpushed local commits, or commits pushed
// after the PR merged. Returns "" when there's nothing at risk.
func (m *model) removeWarning(wt *git.Worktree) string {
	var risks []string
	if wt.Dirty {
		risks = append(risks, "uncommitted changes")
	}
	if wt.Ahead > 0 {
		risks = append(risks, fmt.Sprintf("%d unpushed", wt.Ahead))
	}
	// Commits after a merged PR are genuinely unmerged work.
	if d, ok := m.details[wt.Path]; ok && d.pr != nil && d.pr.State == "MERGED" && d.postMerge > 0 {
		risks = append(risks, fmt.Sprintf("%d after merge", d.postMerge))
	}
	if len(risks) == 0 {
		return ""
	}
	return " [" + strings.Join(risks, ", ") + "!]"
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
	case d.commitErr != nil || len(d.commits) == 0:
		commitLine = pathStyle.Render("  (no commits yet)")
	default:
		c := d.last()
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
