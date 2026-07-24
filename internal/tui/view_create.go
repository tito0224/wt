package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/johnlotito/wt/internal/config"
	"github.com/johnlotito/wt/internal/git"
)

// updateCreate handles the worktree-name input view.
func (m *model) updateCreate(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.Type {
	case tea.KeyCtrlC:
		m.quit = true
		return m, tea.Quit
	case tea.KeyEsc:
		m.view = viewList
		m.create.buf = ""
		return m, nil
	case tea.KeyEnter:
		return m.doCreate()
	case tea.KeyBackspace:
		if m.create.buf != "" {
			r := []rune(m.create.buf)
			m.create.buf = string(r[:len(r)-1])
		}
		return m, nil
	case tea.KeyRunes, tea.KeySpace:
		if key.Type == tea.KeySpace {
			m.create.buf += " "
		} else {
			m.create.buf += string(key.Runes)
		}
		return m, nil
	}
	return m, nil
}

func (m *model) viewCreateRender() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("create worktree") + "\n\n")
	b.WriteString(helpStyle.Render("name (base branch, dir & prefix come from config)") + "\n")
	b.WriteString(filterStyle.Render(m.create.buf+"▏") + "\n")

	// Preview the resulting branch/path when a name has been typed.
	if name := strings.TrimSpace(m.create.buf); name != "" {
		if top, err := git.TopLevel(m.dir); err == nil {
			if cfg, err := config.Load(top); err == nil {
				branch := cfg.Create.BranchName(name)
				path := abbrevHome(cfg.Create.WorktreeDir(top, name))
				b.WriteString("\n" + pathStyle.Render("→ "+branch+"  "+path) + "\n")
			}
		}
	}

	b.WriteString("\n" + m.footer("enter create · esc cancel"))
	return b.String()
}
