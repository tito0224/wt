package tui

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/tito0224/wt/internal/git"
	"github.com/tito0224/wt/internal/ops"
)

// enterMove initializes and opens the move screen for the given worktree.
func (m *model) enterMove(wt git.Worktree) (tea.Model, tea.Cmd) {
	m.view = viewMove
	m.message = ""

	in := textinput.New()
	in.Prompt = ""
	in.Placeholder = wt.Path
	focusCmd := in.Focus()

	m.move = moveState{input: in, wt: wt}
	return m, focusCmd
}

// updateMove handles keys on the move screen.
func (m *model) updateMove(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.Type {
	case tea.KeyCtrlC:
		m.quit = true
		return m, tea.Quit
	case tea.KeyEsc:
		m.view = viewList
		m.move = moveState{}
		return m, nil
	case tea.KeyEnter:
		return m.doMove()
	}

	before := m.move.input.Value()
	var cmd tea.Cmd
	m.move.input, cmd = m.move.input.Update(key)
	if m.move.input.Value() != before {
		m.message = ""
	}
	return m, cmd
}

// doMove kicks off asynchronous worktree move.
func (m *model) doMove() (tea.Model, tea.Cmd) {
	newPath := strings.TrimSpace(m.move.input.Value())
	if newPath == "" {
		m.setErr("path required")
		return m, nil
	}

	wt := m.move.wt
	m.view = viewList
	m.move = moveState{}
	dir := m.dir

	cmd := func() tea.Msg {
		top, err := git.MainDir(dir)
		if err != nil {
			return moveResultMsg{err: err}
		}
		if err := ops.MoveWorktree(top, wt, newPath); err != nil {
			return moveResultMsg{err: err}
		}
		wts, _ := git.ListWorktrees(top)
		return moveResultMsg{path: newPath, wts: wts}
	}
	return m, tea.Batch(m.startBusy("moving…"), cmd)
}

// handleMoveResult applies the outcome of an async move.
func (m *model) handleMoveResult(msg moveResultMsg) (tea.Model, tea.Cmd) {
	m.spinner.active = false
	m.spinner.msg = ""
	if msg.err != nil && msg.wts == nil {
		m.setErr(msg.err.Error())
		return m, nil
	}
	if msg.wts != nil {
		m.all = msg.wts
	}
	m.list.filter = ""
	m.recompute()
	m.selectByPath(msg.path)
	if msg.err != nil {
		m.setErr(fmt.Sprintf("moved, but %v", msg.err))
		return m, m.scheduleDetail()
	}
	m.setMsg(fmt.Sprintf("moved to %s", abbrevHome(msg.path)))
	return m, m.scheduleDetail()
}

// viewMoveRender renders the move screen.
func (m *model) viewMoveRender() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("move worktree") + "\n\n")

	wt := m.move.wt
	b.WriteString(labelStyle.Render(wt.DisplayLabel()) + "\n")
	b.WriteString(pathStyle.Render("  current:  "+abbrevHome(wt.Path)) + "\n")
	b.WriteString(helpStyle.Render("  new path: ") + m.move.input.View() + "\n")

	if p := strings.TrimSpace(m.move.input.Value()); p != "" {
		abs, err := filepath.Abs(p)
		if err == nil {
			b.WriteString(pathStyle.Render("  preview:  "+abbrevHome(abs)) + "\n")
		}
	}

	b.WriteString("\n" + m.footer("esc cancel · enter move"))
	return b.String()
}
