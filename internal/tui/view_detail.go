package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/tito0224/wt/internal/forge"
	"github.com/tito0224/wt/internal/git"
)

// updateDetail handles keys on the full-detail screen. esc/tab/left return to
// the list; the worktree actions (cd/open/remove) work here too, operating on
// the worktree being viewed.
func (m *model) updateDetail(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.Type {
	case tea.KeyCtrlC:
		m.quit = true
		return m, tea.Quit
	case tea.KeyEsc, tea.KeyLeft, tea.KeyTab:
		m.view = viewList
		return m, nil
	case tea.KeyEnter:
		return m.runAction("cd")
	case tea.KeyRunes:
		switch string(key.Runes) {
		case "q":
			m.quit = true
			return m, tea.Quit
		case "h":
			m.view = viewList
		case "o":
			return m.runAction("open")
		case "e":
			return m.quickLaunch()
		case "E":
			return m.enterLaunch()
		case "p":
			return m.runAction("openpr")
		case "d":
			return m.runAction("diff")
		case "D":
			return m.runAction("difftool")
		case "m":
			return m.runAction("move")
		case "y":
			return m.runAction("yank")
		case "r":
			return m.runAction("remove")
		case "R":
			return m.startRefresh()
		case "?":
			m.helpReturn = viewDetail
			m.view = viewHelp
		}
	}
	return m, nil
}

// updateConfirm handles the confirmation prompts:
//
//   - "remove":       y = remove worktree, b = remove worktree + branch,
//     n/esc = cancel.
//   - "force-branch": y = force-delete the leftover unmerged branch,
//     n/esc = keep it.
func (m *model) updateConfirm(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	if key.Type == tea.KeyCtrlC {
		m.quit = true
		return m, tea.Quit
	}
	which := m.list.confirm
	choice := strings.ToLower(string(key.Runes))

	switch which {
	case "remove":
		switch choice {
		case "y":
			m.list.confirm = ""
			return m.doRemove(false, false)
		case "b":
			m.list.confirm = ""
			return m.doRemove(true, false)
		default:
			m.list.confirm = ""
			m.setMsg("cancelled")
		}
	case "force-branch":
		m.list.confirm = ""
		if choice == "y" {
			return m.doForceDeleteBranch()
		}
		m.pendingBranch = ""
		m.setMsg("kept branch")
	default:
		m.list.confirm = ""
	}
	return m, nil
}

func (m *model) viewDetailRender() string {
	wt, ok := m.selected()
	if !ok {
		return ""
	}
	var b strings.Builder

	// Header: branch + markers, then path.
	b.WriteString(titleStyle.Render(wt.DisplayLabel()) + "\n")
	b.WriteString(pathStyle.Render(wt.Path) + "\n\n")

	// Branch status block.
	b.WriteString(headerStyle.Render("status") + "\n")
	for _, line := range m.detailStatusLines(wt) {
		b.WriteString("  " + line + "\n")
	}
	b.WriteString(m.detailChangedFiles(wt))

	// PR block, when known.
	if d, ok := m.details[wt.Path]; ok && d.prChecked && d.pr != nil {
		b.WriteString("\n" + headerStyle.Render("pull request") + "\n")
		b.WriteString("  " + prBadge(d.pr) + "  " + d.pr.Title + "\n")
		b.WriteString("  " + pathStyle.Render(d.pr.URL) + "\n")
	}

	// Recent commits.
	b.WriteString("\n" + headerStyle.Render("recent commits") + "\n")
	b.WriteString(m.detailLog(wt))

	// Shared key bar (with confirm/message/spinner handling).
	b.WriteString("\n" + m.keyBarFooter(m.keyBarHints(viewDetail, m.selectedPR() != nil, m.selectedDirty())))
	return b.String()
}

// detailStatusLines returns the branch-status lines for the detail screen.
func (m *model) detailStatusLines(wt git.Worktree) []string {
	var lines []string

	// Working-tree line: clean, or staged/modified/untracked counts from the
	// loaded porcelain status (falls back to the coarse Dirty flag while
	// loading).
	lines = append(lines, "working tree:  "+m.workingTreeSummary(wt))

	if wt.Detached {
		lines = append(lines, "head:          "+pathStyle.Render("(detached)"))
	}

	// Upstream (remote tracking branch): highlight unpushed local commits.
	if wt.HasUpstream {
		up := fmt.Sprintf("ahead %d, behind %d", wt.Ahead, wt.Behind)
		if wt.Ahead > 0 {
			up = dirtyStyle.Render(fmt.Sprintf("%d unpushed", wt.Ahead))
			if wt.Behind > 0 {
				up += pathStyle.Render(fmt.Sprintf(", behind %d", wt.Behind))
			}
		}
		lines = append(lines, "upstream:      "+up)
	} else if !wt.Detached {
		lines = append(lines, "upstream:      "+pathStyle.Render("(not pushed)"))
	}

	// Merge status: resolved from the PR (authoritative) combined with the
	// local commits-ahead-of-base count.
	if ml := m.mergeStatusLine(wt); ml != "" {
		lines = append(lines, "merge:         "+ml)
	}

	if wt.Locked {
		lines = append(lines, "locked:        yes")
	}
	return lines
}

// mergeStatusLine describes whether the branch's work is merged, using the PR
// as the source of truth (squash/rebase merges rewrite SHAs, so the local
// commit count alone is unreliable). Cases:
//
//   - PR merged, no commits ahead of base -> "merged (#N)".
//   - PR merged, commits ahead of base    -> "merged (#N), but K commits after
//     merge" (work pushed after the PR merged; genuinely unmerged).
//   - PR open/closed                      -> reflects the PR state.
//   - No PR info                          -> neutral "K commits ahead of <base>"
//     fallback (never asserts "unmerged", since we can't know locally).
func (m *model) mergeStatusLine(wt git.Worktree) string {
	base := wt.BaseBranch
	if base == "" {
		base = "default branch"
	}
	pr := m.prFor(wt)
	d := m.details[wt.Path]

	if pr != nil {
		switch pr.State {
		case "MERGED":
			if d.postMerge == 0 {
				return cleanStyle.Render(fmt.Sprintf("merged (#%d)", pr.Number))
			}
			return cleanStyle.Render(fmt.Sprintf("merged (#%d)", pr.Number)) + ", " +
				dirtyStyle.Render(fmt.Sprintf("%d commit(s) after merge", d.postMerge))
		case "CLOSED":
			s := errorStyle.Render(fmt.Sprintf("PR #%d closed (not merged)", pr.Number))
			if wt.AheadOfBase > 0 {
				s += pathStyle.Render(fmt.Sprintf(", %d ahead of %s", wt.AheadOfBase, base))
			}
			return s
		case "OPEN":
			return pathStyle.Render(fmt.Sprintf("PR #%d open, %d commit(s) ahead of %s",
				pr.Number, wt.AheadOfBase, base))
		}
	}

	// No PR data: neutral, factual count. Not a claim about merge state.
	if wt.AheadOfBase > 0 {
		return pathStyle.Render(fmt.Sprintf("%d commit(s) ahead of %s", wt.AheadOfBase, base))
	}
	return ""
}

// prFor returns the loaded PR for a worktree (nil when unknown).
func (m *model) prFor(wt git.Worktree) *forge.PR {
	if d, ok := m.details[wt.Path]; ok {
		return d.pr
	}
	return nil
}

// workingTreeSummary describes the working tree using the loaded porcelain
// status when available, otherwise the coarse worktree Dirty flag.
func (m *model) workingTreeSummary(wt git.Worktree) string {
	d, ok := m.details[wt.Path]
	if !ok || d.statusErr != nil {
		if wt.Dirty {
			return dirtyStyle.Render("dirty")
		}
		return cleanStyle.Render("clean")
	}
	st := d.status
	if st.Clean() {
		return cleanStyle.Render("clean")
	}
	var parts []string
	if st.Staged > 0 {
		parts = append(parts, cleanStyle.Render(fmt.Sprintf("%d staged", st.Staged)))
	}
	if st.Modified > 0 {
		parts = append(parts, dirtyStyle.Render(fmt.Sprintf("%d modified", st.Modified)))
	}
	if st.Untracked > 0 {
		parts = append(parts, pathStyle.Render(fmt.Sprintf("%d untracked", st.Untracked)))
	}
	return strings.Join(parts, ", ")
}

// detailChangedFilesMax caps how many changed files the status block lists.
const detailChangedFilesMax = 8

// detailChangedFiles renders a short, capped list of changed files (with their
// two-character porcelain codes). Returns "" when clean or not yet loaded.
func (m *model) detailChangedFiles(wt git.Worktree) string {
	d, ok := m.details[wt.Path]
	if !ok || d.statusErr != nil || d.status.Clean() {
		return ""
	}
	var b strings.Builder
	files := d.status.Files
	shown := files
	if len(shown) > detailChangedFilesMax {
		shown = shown[:detailChangedFilesMax]
	}
	for _, f := range shown {
		b.WriteString("    " + statusCodeStyle(f.Code).Render(f.Code) + " " +
			trunc(f.Path, max(1, m.width-8)) + "\n")
	}
	if extra := len(files) - len(shown); extra > 0 {
		b.WriteString("    " + pathStyle.Render(fmt.Sprintf("… and %d more", extra)) + "\n")
	}
	return b.String()
}

// statusCodeStyle colors a porcelain XY code: untracked dim, staged green,
// otherwise (unstaged modifications) yellow.
func statusCodeStyle(code string) lipgloss.Style {
	switch {
	case code == "??":
		return pathStyle
	case code[0] != ' ':
		return cleanStyle
	default:
		return dirtyStyle
	}
}

// detailLog renders the recent commit log (or a placeholder while loading).
func (m *model) detailLog(wt git.Worktree) string {
	d, ok := m.details[wt.Path]
	switch {
	case !ok:
		return pathStyle.Render("  loading…") + "\n"
	case d.commitErr != nil || len(d.commits) == 0:
		return pathStyle.Render("  (no commits yet)") + "\n"
	}
	var b strings.Builder
	for _, c := range d.commits {
		hash := currentStyle.Render(c.ShortHash)
		meta := pathStyle.Render(fmt.Sprintf("%s · %s", c.RelDate, c.Author))
		subj := trunc(c.Subject, max(1, m.width-4))
		b.WriteString("  " + hash + "  " + subj + "\n")
		b.WriteString("  " + strings.Repeat(" ", len(c.ShortHash)) + "  " + meta + "\n")
	}
	return b.String()
}
