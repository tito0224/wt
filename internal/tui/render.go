package tui

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/johnlotito/wt/internal/git"
)

// clampFrame guarantees the rendered frame never exceeds the terminal size:
// every line is hard-truncated to the terminal width, and the number of lines
// is capped to the terminal height. This prevents soft-wrapping (which
// desyncs Bubble Tea's line accounting and produces garbage frames on resize).
func (m *model) clampFrame(s string) string {
	lines := strings.Split(s, "\n")
	if m.height > 0 && len(lines) > m.height {
		lines = lines[:m.height]
	}
	if m.width > 0 {
		for i, ln := range lines {
			if lipgloss.Width(ln) > m.width {
				// ansi.Truncate respects escape sequences (won't cut mid-code).
				lines[i] = ansi.Truncate(ln, m.width, "")
			}
		}
	}
	return strings.Join(lines, "\n")
}

// layout describes the computed column widths for the current terminal size.
type layout struct {
	branchW  int
	statusW  int
	pathW    int // 0 means the PATH column is dropped
	showPath bool
}

// Fixed spacing: "> " row prefix (2) + two 2-space column separators (4).
const rowOverhead = 2 + 2 + 2

// computeLayout allocates horizontal space with this priority:
//  1. STATUS is never truncated (short, known values).
//  2. PATH shrinks first, and is dropped entirely if too little room remains.
//  3. BRANCH is truncated only as a last resort, when STATUS + a minimal BRANCH
//     don't even fit.
func (m *model) computeLayout() layout {
	// Natural widths from content.
	branchNat := lipgloss.Width("BRANCH")
	statusNat := lipgloss.Width("STATUS")
	pathNat := lipgloss.Width("PATH")
	for _, r := range m.rows {
		if w := lipgloss.Width(r.wt.DisplayLabel()); w > branchNat {
			branchNat = w
		}
		if w := lipgloss.Width(plainStatus(r.wt)); w > statusNat {
			statusNat = w
		}
		if w := lipgloss.Width(abbrevHome(r.wt.Path)); w > pathNat {
			pathNat = w
		}
	}

	avail := m.width - rowOverhead
	if avail < 1 {
		avail = 1
	}

	// STATUS is protected from truncation relative to the other columns, but at
	// pathologically narrow widths (STATUS alone wider than the terminal) even
	// it must yield; reserve at least minBranch for the branch column.
	const minBranch = 6
	statusW := statusNat
	if statusW > avail-minBranch {
		statusW = max(1, avail-minBranch)
	}
	l := layout{statusW: statusW}

	// Space left for BRANCH + PATH after STATUS.
	rest := avail - statusW
	if rest < 0 {
		rest = 0
	}

	// Last resort: not even STATUS + a minimal branch fits -> truncate BRANCH.
	if rest < minBranch {
		l.branchW = max(1, rest)
		l.showPath = false
		return l
	}

	// Try to give BRANCH its natural width, capped so a single huge branch
	// can't starve PATH.
	branchW := branchNat
	if branchW > 40 {
		branchW = 40
	}
	if branchW > rest {
		branchW = rest
	}
	l.branchW = branchW

	// PATH gets whatever remains. Drop it if there's too little to be useful.
	const minPath = 12
	pathW := rest - branchW
	if pathW >= minPath {
		l.showPath = true
		l.pathW = min(pathW, pathNat)
	}
	return l
}

// footer renders the shared status/help line. While a git operation is in
// flight, the spinner + label replace the usual message/help line.
func (m *model) footer(help string) string {
	var b strings.Builder
	if m.spinner.active {
		b.WriteString(titleStyle.Render(spinnerFrames[m.spinner.frame]) + " " + helpStyle.Render(m.spinner.msg))
		return b.String()
	}
	if m.message != "" {
		if m.isErr {
			b.WriteString(errorStyle.Render("✗ "+m.message) + "\n")
		} else {
			b.WriteString(cleanStyle.Render("✓ "+m.message) + "\n")
		}
	}
	b.WriteString(helpStyle.Render(help))
	return b.String()
}

// renderRow renders one table row. The selected row is drawn as plain text
// (no per-cell color) wrapped in reverse video, which stays readable on any
// theme. Non-selected rows get per-cell semantic coloring. Both share the same
// widths from the layout, so columns always align and never overflow.
func (m *model) renderRow(r row, selected bool, lay layout) string {
	label := trunc(r.wt.DisplayLabel(), lay.branchW)
	status := trunc(plainStatus(r.wt), lay.statusW)
	labelW := lipgloss.Width(label)
	statusFits := status == plainStatus(r.wt)

	if selected {
		line := "> " + pad(label, lay.branchW) + "  " + pad(status, lay.statusW)
		if lay.showPath {
			line += "  " + truncPath(r.wt.Path, lay.pathW)
		}
		return selectedStyle.Render(line)
	}

	// Colored cells, padded to width using plain-text display width. When the
	// label was truncated we can't recolor by original match indices, so fall
	// back to plain (still current-colored) rendering. Colored status only when
	// it wasn't clipped (clipping breaks per-tag coloring).
	branchCell := colorLabelTrunc(r, label) + strings.Repeat(" ", max(0, lay.branchW-labelW))
	statusColored := coloredStatus(r.wt)
	if !statusFits {
		statusColored = status
	}
	statusCell := statusColored + strings.Repeat(" ", max(0, lay.statusW-lipgloss.Width(status)))
	out := "  " + branchCell + "  " + statusCell
	if lay.showPath {
		out += "  " + pathStyle.Render(truncPath(r.wt.Path, lay.pathW))
	}
	return out
}

// colorLabelTrunc colors a (possibly truncated) label: cyan for the current
// worktree, otherwise fuzzy-match highlighting when the label is intact.
func colorLabelTrunc(r row, truncated string) string {
	if r.wt.Current {
		return currentStyle.Render(truncated)
	}
	// If truncation changed the text, match indices no longer line up.
	if truncated != r.wt.DisplayLabel() {
		return truncated
	}
	return highlight(r.wt.Label(), r.matched)
}

// trunc right-truncates s to a display width, appending "…" when clipped.
func trunc(s string, width int) string {
	if width <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= width {
		return s
	}
	r := []rune(s)
	for len(r) > 0 && lipgloss.Width(string(r))+1 > width {
		r = r[:len(r)-1]
	}
	return string(r) + "…"
}

// abbrevHome replaces the user's home directory prefix with "~".
func abbrevHome(p string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return p
	}
	if p == home {
		return "~"
	}
	if strings.HasPrefix(p, home+string(os.PathSeparator)) {
		return "~" + p[len(home):]
	}
	return p
}

// pad right-pads s with spaces to the given display width.
func pad(s string, width int) string {
	w := lipgloss.Width(s)
	if w >= width {
		return s
	}
	return s + strings.Repeat(" ", width-w)
}

// plainStatus is the uncolored status string, used for width calculations.
func plainStatus(w git.Worktree) string { return w.StatusString() }

// coloredStatus renders status tags with per-tag colors.
func coloredStatus(w git.Worktree) string {
	tags := w.StatusTags()
	parts := make([]string, len(tags))
	for i, t := range tags {
		switch {
		case t == "dirty":
			parts[i] = dirtyStyle.Render(t)
		case t == "unmerged":
			parts[i] = unmergedStyle.Render(t)
		case t == "clean":
			parts[i] = cleanStyle.Render(t)
		default:
			parts[i] = pathStyle.Render(t)
		}
	}
	return strings.Join(parts, ", ")
}

// truncPath abbreviates $HOME to ~ and left-ellipsizes to fit width, keeping
// the tail (the worktree's own directory name) visible.
func truncPath(p string, width int) string {
	p = abbrevHome(p)
	if width <= 0 {
		return ""
	}
	if lipgloss.Width(p) <= width {
		return p
	}
	r := []rune(p)
	for len(r) > 0 && lipgloss.Width(string(r))+1 > width {
		r = r[1:]
	}
	return "…" + string(r)
}

// highlight bolds the matched rune positions in s.
func highlight(s string, matched []int) string {
	if len(matched) == 0 {
		return s
	}
	set := make(map[int]bool, len(matched))
	for _, i := range matched {
		set[i] = true
	}
	var b strings.Builder
	for i, r := range []rune(s) {
		if set[i] {
			b.WriteString(matchStyle.Render(string(r)))
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// printStatus writes a worktree's detailed status to stdout (used when the
// picker exits with the "status" action).
func printStatus(w git.Worktree) {
	fmt.Printf("%s\n  path:     %s\n", w.Label(), w.Path)
	if !w.Bare {
		fmt.Printf("  dirty:    %t\n  unmerged: %t\n", w.Dirty, w.Unmerged)
		if w.HasUpstream {
			fmt.Printf("  ahead:    %d\n  behind:   %d\n", w.Ahead, w.Behind)
		}
	}
}
