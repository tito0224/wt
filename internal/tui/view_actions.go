package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

type action struct {
	key   string
	label string
	id    string
}

var actions = []action{
	{"c", "cd here (current shell)", "cd"},
	{"o", "Open in new terminal tab", "open"},
	{"s", "Show detailed status", "status"},
	{"r", "Remove worktree", "remove"},
}

func (m *model) updateActions(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.Type {
	case tea.KeyCtrlC:
		m.quit = true
		return m, tea.Quit
	case tea.KeyEsc, tea.KeyLeft:
		m.view = viewList
		return m, nil
	case tea.KeyUp:
		if m.menu.cursor > 0 {
			m.menu.cursor--
		}
		return m, nil
	case tea.KeyDown:
		if m.menu.cursor < len(actions)-1 {
			m.menu.cursor++
		}
		return m, nil
	case tea.KeyEnter:
		return m.runAction(actions[m.menu.cursor].id)
	case tea.KeyRunes:
		for _, a := range actions {
			if string(key.Runes) == a.key {
				return m.runAction(a.id)
			}
		}
	}
	return m, nil
}

// updateConfirm handles the y/n confirmation prompt (currently only for
// removal), shown over the list view.
func (m *model) updateConfirm(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	if key.Type == tea.KeyCtrlC {
		m.quit = true
		return m, tea.Quit
	}
	switch strings.ToLower(string(key.Runes)) {
	case "y":
		action := m.menu.confirm
		m.menu.confirm = ""
		if action == "remove" {
			return m.doRemove()
		}
	default:
		// Anything else (n, esc, etc.) cancels.
		m.menu.confirm = ""
		m.setMsg("cancelled")
	}
	return m, nil
}

func (m *model) viewActionsRender() string {
	wt := m.selected()
	var b strings.Builder
	b.WriteString(titleStyle.Render("actions: "+wt.Label()) + "\n")
	b.WriteString(pathStyle.Render(wt.Path) + "\n\n")
	for i, a := range actions {
		label := fmt.Sprintf("  [%s] %s", a.key, a.label)
		if i == m.menu.cursor {
			label = selectedStyle.Render(fmt.Sprintf("> [%s] %s", a.key, a.label))
		}
		b.WriteString(label + "\n")
	}
	b.WriteString("\n" + m.footer("↑/↓ move · enter run · esc back"))
	return b.String()
}
