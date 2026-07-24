package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/johnlotito/wt/internal/launcher"
	"github.com/johnlotito/wt/internal/terminal"
)

// quickLaunch opens the selected worktree in the default launcher without a
// menu: the configured default (config/WT_LAUNCHER) if available, otherwise the
// single available launcher, otherwise it falls back to the picker so the user
// can choose.
func (m *model) quickLaunch() (tea.Model, tea.Cmd) {
	wt, ok := m.selected()
	if !ok {
		return m, nil
	}
	// Remember where the launch flow started (list or detail) so launching —
	// or cancelling out of the fallback picker — lands back there, mirroring
	// helpReturn. Launching never removes the worktree, so the origin view is
	// always still valid.
	m.launchReturn = m.view
	items := launcher.Available()
	if len(items) == 0 {
		m.setErr("no editors or agents found (install code, cursor, zed, claude, or opencode)")
		return m, nil
	}

	// Prefer the configured default when it's available.
	if m.defaultLauncher != "" {
		for _, l := range items {
			if l.Name() == m.defaultLauncher {
				return m.launchInto(l, wt.Path, wt.Label())
			}
		}
	}
	// No (available) default: one option launches directly, several open the picker.
	if len(items) == 1 {
		return m.launchInto(items[0], wt.Path, wt.Label())
	}
	return m.enterLaunch()
}

// enterLaunch opens the launcher submenu for the selected worktree, listing the
// available editors/agents. If none are installed it surfaces a message instead.
func (m *model) enterLaunch() (tea.Model, tea.Cmd) {
	return m.enterLaunchWith(launcher.Available())
}

// enterLaunchWith is enterLaunch with an explicit launcher set — split out so
// tests can inject fixtures instead of probing the host for installed editors.
func (m *model) enterLaunchWith(items []launcher.Launcher) (tea.Model, tea.Cmd) {
	wt, ok := m.selected()
	if !ok {
		return m, nil
	}
	if len(items) == 0 {
		m.setErr("no editors or agents found (install code, cursor, zed, claude, or opencode)")
		return m, nil
	}
	// Remember where the submenu was opened from (list or detail) so esc and
	// launching return there, mirroring helpReturn. (When we got here via
	// quickLaunch's fallback, m.view is still the origin view, so this is the
	// same value quickLaunch recorded.)
	m.launchReturn = m.view
	m.launch = launchState{
		items:   items,
		wtPath:  wt.Path,
		wtLabel: wt.Label(),
	}
	m.view = viewLaunch
	m.message = ""
	return m, nil
}

// updateLaunch handles keys in the launcher submenu.
func (m *model) updateLaunch(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.Type {
	case tea.KeyCtrlC:
		m.quit = true
		return m, tea.Quit
	case tea.KeyEsc, tea.KeyLeft:
		m.view = m.launchReturn
		return m, nil
	case tea.KeyUp:
		if m.launch.selectedRow > 0 {
			m.launch.selectedRow--
		}
		return m, nil
	case tea.KeyDown:
		if m.launch.selectedRow < len(m.launch.items)-1 {
			m.launch.selectedRow++
		}
		return m, nil
	case tea.KeyEnter:
		return m.doLaunch()
	case tea.KeyRunes:
		switch string(key.Runes) {
		case "j":
			if m.launch.selectedRow < len(m.launch.items)-1 {
				m.launch.selectedRow++
			}
		case "k":
			if m.launch.selectedRow > 0 {
				m.launch.selectedRow--
			}
		case "q":
			m.view = m.launchReturn
		}
	}
	return m, nil
}

// doLaunch runs the highlighted launcher from the picker.
func (m *model) doLaunch() (tea.Model, tea.Cmd) {
	if m.launch.selectedRow < 0 || m.launch.selectedRow >= len(m.launch.items) {
		return m, nil
	}
	l := m.launch.items[m.launch.selectedRow]
	return m.launchInto(l, m.launch.wtPath, m.launch.wtLabel)
}

// launchInto runs launcher l for the worktree at path, then returns to the
// view the launch flow started from (list or detail — the worktree still
// exists either way, so both success and error land back where the user was).
// Terminal-kind launchers open a new terminal tab via the resolved terminal
// adapter; GUI/URL launchers run directly.
func (m *model) launchInto(l launcher.Launcher, path, label string) (tea.Model, tea.Cmd) {
	m.view = m.launchReturn

	var term terminal.Adapter
	if l.Kind() == launcher.KindTerminal {
		// Resolve a terminal only when needed; report clearly if none is found.
		t, err := terminal.Resolve(m.terminalName)
		if err != nil {
			m.setErr(err.Error())
			return m, nil
		}
		term = t
	}

	if err := launcher.Launch(l, path, term); err != nil {
		m.setErr(err.Error())
		return m, nil
	}
	m.setMsg(fmt.Sprintf("opened %s in %s", label, l.Title()))
	return m, nil
}

func (m *model) viewLaunchRender() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("open "+m.launch.wtLabel) + "\n")
	b.WriteString(pathStyle.Render(abbrevHome(m.launch.wtPath)) + "\n\n")

	for i, l := range m.launch.items {
		icon := l.Icon()
		if icon != "" {
			icon += "  "
		}
		if i == m.launch.selectedRow {
			// Whole line in reverse video with plain (unstyled) text — including
			// the kind hint — so it stays readable on any theme and the hint
			// doesn't disappear as the cursor moves.
			b.WriteString(selectedStyle.Render("> "+icon+l.Title()+kindHintPlain(l.Kind())) + "\n")
		} else {
			b.WriteString("  " + icon + l.Title() + kindHint(l.Kind()) + "\n")
		}
	}

	b.WriteString("\n" + m.footer("↑/↓ select · enter open · esc cancel"))
	return b.String()
}

// kindHint returns a short dim suffix describing how a launcher opens.
func kindHint(k launcher.Kind) string {
	if s := kindHintPlain(k); s != "" {
		return pathStyle.Render(s)
	}
	return ""
}

// kindHintPlain is the unstyled kind hint text (for use inside reverse-video
// selected rows, where nested styling would look wrong).
func kindHintPlain(k launcher.Kind) string {
	switch k {
	case launcher.KindTerminal:
		return "  (new terminal)"
	case launcher.KindURL:
		return "  (browser)"
	default:
		return ""
	}
}
