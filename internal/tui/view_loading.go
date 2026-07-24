package tui

import "strings"

// viewLoadingRender shows the spinner while the initial worktree enumeration
// is in progress.
func (m *model) viewLoadingRender() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("git worktrees") + "\n\n")
	b.WriteString(m.spinner.model.View() + " " +
		helpStyle.Render("loading worktrees…"))
	return b.String()
}
