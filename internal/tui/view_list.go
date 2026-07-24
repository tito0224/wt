package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

func (m *model) updateList(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.list.searching {
		return m.updateSearch(key)
	}
	return m.updateNormal(key)
}

// updateNormal handles keys in normal mode, where single letters are
// shortcuts and '/' enters search mode.
func (m *model) updateNormal(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.message = ""
	switch key.Type {
	case tea.KeyCtrlC:
		m.quit = true
		return m, tea.Quit
	case tea.KeyEsc:
		// First esc clears an active filter; otherwise quit.
		if m.list.filter != "" {
			m.list.filter = ""
			m.recompute()
			return m, nil
		}
		m.quit = true
		return m, tea.Quit
	case tea.KeyUp:
		m.moveCursor(-1)
		return m, nil
	case tea.KeyDown:
		m.moveCursor(1)
		return m, nil
	case tea.KeyPgUp:
		m.moveCursor(-m.visibleRows())
		return m, nil
	case tea.KeyPgDown:
		m.moveCursor(m.visibleRows())
		return m, nil
	case tea.KeyHome:
		m.cursor = 0
		m.clampScroll()
		return m, nil
	case tea.KeyEnd:
		m.cursor = len(m.rows) - 1
		m.clampScroll()
		return m, nil
	case tea.KeyEnter:
		if m.selected() != nil {
			return m.runAction("cd")
		}
		return m, nil
	case tea.KeyTab, tea.KeyRight:
		if m.selected() != nil {
			m.view = viewActions
			m.menu.cursor = 0
		}
		return m, nil
	case tea.KeyRunes:
		switch string(key.Runes) {
		case "/":
			m.list.searching = true
			return m, nil
		case "j":
			m.moveCursor(1)
		case "k":
			m.moveCursor(-1)
		case "g":
			m.cursor = 0
			m.clampScroll()
		case "G":
			m.cursor = len(m.rows) - 1
			m.clampScroll()
		case "o":
			return m.runAction("open")
		case "a":
			m.view = viewCreate
			m.create.buf = ""
			m.message = ""
		case "s":
			return m.runAction("status")
		case "r":
			return m.runAction("remove")
		case "q":
			m.quit = true
			return m, tea.Quit
		}
		return m, nil
	}
	return m, nil
}

// updateSearch handles keys while typing a filter query. Enter/esc leave
// search mode (keeping the current filter); a further esc in normal mode
// clears it.
func (m *model) updateSearch(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.Type {
	case tea.KeyCtrlC:
		m.quit = true
		return m, tea.Quit
	case tea.KeyEsc, tea.KeyEnter:
		m.list.searching = false
		return m, nil
	case tea.KeyUp:
		m.moveCursor(-1)
		return m, nil
	case tea.KeyDown:
		m.moveCursor(1)
		return m, nil
	case tea.KeyBackspace:
		if m.list.filter != "" {
			r := []rune(m.list.filter)
			m.list.filter = string(r[:len(r)-1])
			m.recompute()
		}
		return m, nil
	case tea.KeyRunes, tea.KeySpace:
		m.message = ""
		m.list.filter += string(key.Runes)
		if key.Type == tea.KeySpace {
			m.list.filter += " "
		}
		m.recompute()
		return m, nil
	}
	return m, nil
}

func (m *model) viewListRender() string {
	var b strings.Builder
	lay := m.computeLayout()

	// Title + count.
	title := fmt.Sprintf("git worktrees (%d)", len(m.all))
	if m.list.filter != "" {
		title += fmt.Sprintf("  —  %d match", len(m.rows))
	}
	b.WriteString(titleStyle.Render(title) + "\n")

	// Filter/search line.
	switch {
	case m.list.searching:
		// Show a cursor block while actively typing the query.
		b.WriteString(filterStyle.Render("/"+m.list.filter+"▏") + "\n")
	case m.list.filter != "":
		b.WriteString(filterStyle.Render("/"+m.list.filter) + helpStyle.Render("  (filtered)") + "\n")
	default:
		b.WriteString(helpStyle.Render("press / to search") + "\n")
	}

	// Header. Two leading spaces align with the "> " / "  " row prefix.
	header := "  " + pad(trunc("BRANCH", lay.branchW), lay.branchW) + "  " + pad("STATUS", lay.statusW)
	if lay.showPath {
		header += "  PATH"
	}
	b.WriteString(headerStyle.Render(header) + "\n")

	if len(m.rows) == 0 {
		b.WriteString(pathStyle.Render("  (no matches)") + "\n")
	}

	vis := m.visibleRows()
	end := m.offset + vis
	if end > len(m.rows) {
		end = len(m.rows)
	}
	for i := m.offset; i < end; i++ {
		b.WriteString(m.renderRow(m.rows[i], i == m.cursor, lay) + "\n")
	}

	// Scroll indicator.
	if len(m.rows) > vis {
		b.WriteString(helpStyle.Render(fmt.Sprintf("  [%d-%d of %d]", m.offset+1, end, len(m.rows))) + "\n")
	}

	// Confirmation prompt takes over the footer when active.
	if m.menu.confirm == "remove" {
		wt := m.selected()
		name := ""
		if wt != nil {
			name = wt.Label()
		}
		warn := ""
		if wt != nil && wt.Dirty {
			warn = errorStyle.Render(" [has uncommitted changes!]")
		}
		b.WriteString("\n" + errorStyle.Render(fmt.Sprintf("remove %s?", name)) + warn +
			helpStyle.Render("  (y/n)"))
		return b.String()
	}

	// Status bar: async-loaded detail (last commit + PR) for the selection.
	if sb := m.statusBar(); sb != "" {
		b.WriteString("\n" + sb + "\n")
	}

	// Footer: a transient message (or spinner) line, then the key bar.
	b.WriteString("\n")
	if m.spinner.active {
		b.WriteString(titleStyle.Render(spinnerFrames[m.spinner.frame]) + " " +
			helpStyle.Render(m.spinner.msg) + "\n")
	} else if m.message != "" {
		if m.isErr {
			b.WriteString(errorStyle.Render("✗ "+m.message) + "\n")
		} else {
			b.WriteString(cleanStyle.Render("✓ "+m.message) + "\n")
		}
	}
	if m.list.searching {
		b.WriteString(helpStyle.Render("type to filter · enter/esc done"))
	} else {
		b.WriteString(renderKeyBar(listKeyHints))
	}
	return b.String()
}
