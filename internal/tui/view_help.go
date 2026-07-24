package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// helpEntry is one row in the full help overlay.
type helpEntry struct {
	keys string
	desc string
}

// helpEntries is the complete key reference shown by the `?` overlay.
var helpEntries = []helpEntry{
	{"↵ enter", "cd current shell into the worktree"},
	{"a", "add a new worktree"},
	{"o", "open the worktree in a new terminal tab"},
	{"p", "open the pull request in a browser"},
	{"d", "git diff (uncommitted changes)"},
	{"D", "git difftool"},
	{"tab / → / l", "open the details screen"},
	{"esc / tab / h", "leave the details screen"},
	{"r", "remove the worktree (asks to confirm)"},
	{"/", "search / filter"},
	{"j / k / ↑ / ↓", "move selection"},
	{"g / G", "jump to top / bottom"},
	{"?", "toggle this help"},
	{"q / esc", "quit"},
}

// updateHelp handles keys while the help overlay is shown: any of esc/?/q
// closes it (q also quits from the overlay is avoided—esc/? return).
func (m *model) updateHelp(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.Type {
	case tea.KeyCtrlC:
		m.quit = true
		return m, tea.Quit
	case tea.KeyEsc:
		m.view = m.helpReturn
		return m, nil
	case tea.KeyRunes:
		switch string(key.Runes) {
		case "?", "q", "h":
			m.view = m.helpReturn
		}
	}
	return m, nil
}

func (m *model) viewHelpRender() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("keys") + "\n\n")

	// Align the key column.
	maxKey := 0
	for _, e := range helpEntries {
		if n := len([]rune(e.keys)); n > maxKey {
			maxKey = n
		}
	}
	for _, e := range helpEntries {
		keys := keycapStyle.Render(" " + e.keys + " ")
		padTo := maxKey - len([]rune(e.keys))
		b.WriteString("  " + keys + strings.Repeat(" ", padTo+1) +
			labelStyle.Render(e.desc) + "\n")
	}

	b.WriteString("\n" + helpStyle.Render("press ? or esc to close"))
	return b.String()
}
