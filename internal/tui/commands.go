package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/johnlotito/wt/internal/config"
	"github.com/johnlotito/wt/internal/git"
	"github.com/johnlotito/wt/internal/terminal"
)

// loadCmd enumerates worktrees off the main loop, delivering loadResultMsg.
func loadCmd(dir string) tea.Cmd {
	return func() tea.Msg {
		wts, err := git.ListWorktrees(dir)
		return loadResultMsg{wts: wts, err: err}
	}
}

// handleLoadResult applies the outcome of the initial worktree enumeration.
func (m *model) handleLoadResult(msg loadResultMsg) (tea.Model, tea.Cmd) {
	m.spinner.loading = false
	if msg.err != nil {
		m.setErr(msg.err.Error())
		return m, nil
	}
	m.all = msg.wts
	m.recompute()
	// Load detail for the initially-selected worktree.
	return m, m.scheduleDetail()
}

// runAction dispatches a selection-based action (from the list or the action
// submenu). "cd"/"status" quit and hand a result back to Run; "open" launches a
// terminal tab; "remove" opens the confirmation prompt.
func (m *model) runAction(id string) (tea.Model, tea.Cmd) {
	wt := m.selected()
	if wt == nil {
		return m, nil
	}
	switch id {
	case "open":
		adapter, err := terminal.Resolve(m.terminalName)
		if err != nil {
			m.setErr(err.Error())
			m.view = viewList
			return m, nil
		}
		if err := adapter.OpenTab(wt.Path); err != nil {
			m.setErr(err.Error())
			m.view = viewList
			return m, nil
		}
		m.setMsg(fmt.Sprintf("opened %s in new %s tab", wt.Label(), adapter.Name()))
		m.view = viewList
	case "cd", "status":
		m.resultAction = id
		m.resultWT = wt
		return m, tea.Quit
	case "remove":
		// Ask for confirmation before a destructive removal (especially since
		// dirty worktrees are force-removed).
		m.view = viewList
		m.menu.confirm = "remove"
		return m, nil
	}
	return m, nil
}

// doCreate kicks off asynchronous worktree creation from the typed name using
// config defaults. The blocking git work runs in a tea.Cmd so the UI keeps
// repainting (spinner) while it happens; completion arrives as createResultMsg.
func (m *model) doCreate() (tea.Model, tea.Cmd) {
	name := strings.TrimSpace(m.create.buf)
	if name == "" {
		m.setErr("worktree name required")
		return m, nil
	}

	m.view = viewList
	m.create.buf = ""
	dir := m.dir

	cmd := func() tea.Msg {
		top, err := git.TopLevel(dir)
		if err != nil {
			return createResultMsg{err: err}
		}
		branch, path, err := config.CreateWorktree(top, name, "", "")
		if err != nil {
			return createResultMsg{err: err}
		}
		// Refresh from git so the new worktree is fully enriched (status, etc.).
		wts, _ := git.ListWorktrees(top)
		return createResultMsg{branch: branch, path: path, wts: wts}
	}
	return m, tea.Batch(m.startBusy(fmt.Sprintf("creating %s…", name)), cmd)
}

// handleCreateResult applies the outcome of an async creation: on success it
// swaps in the refreshed worktree set, clears the filter, and highlights the
// new worktree; on failure it surfaces the error.
func (m *model) handleCreateResult(msg createResultMsg) (tea.Model, tea.Cmd) {
	m.spinner.active = false
	m.spinner.msg = ""
	if msg.err != nil {
		m.setErr(msg.err.Error())
		return m, nil
	}
	if msg.wts != nil {
		m.all = msg.wts
	}
	m.list.filter = ""
	m.recompute()
	m.selectByPath(msg.path)
	m.setMsg(fmt.Sprintf("created %s at %s", msg.branch, abbrevHome(msg.path)))
	return m, m.scheduleDetail()
}

// doRemove kicks off asynchronous worktree removal after confirmation. The
// blocking git work runs in a tea.Cmd so the spinner can animate; completion
// arrives as removeResultMsg.
func (m *model) doRemove() (tea.Model, tea.Cmd) {
	wt := m.selected()
	if wt == nil {
		return m, nil
	}
	if wt.Current {
		m.setErr("refusing to remove the worktree you're currently in")
		return m, nil
	}
	force := wt.Dirty
	label := wt.Label()
	path := wt.Path
	dir := m.dir

	cmd := func() tea.Msg {
		err := git.Remove(dir, path, force)
		return removeResultMsg{label: label, path: path, force: force, err: err}
	}
	return m, tea.Batch(m.startBusy(fmt.Sprintf("removing %s…", label)), cmd)
}

// handleRemoveResult applies the outcome of an async removal: on success it
// drops the worktree from the list; on failure it surfaces the error.
func (m *model) handleRemoveResult(msg removeResultMsg) (tea.Model, tea.Cmd) {
	m.spinner.active = false
	m.spinner.msg = ""
	if msg.err != nil {
		m.setErr(msg.err.Error())
		return m, nil
	}
	note := ""
	if msg.force {
		note = " (forced, discarded changes)"
	}
	delete(m.details, msg.path)
	m.removeFromAll(msg.path)
	m.setMsg(fmt.Sprintf("removed %s%s", msg.label, note))
	// The cursor now points at a different worktree; load its detail.
	return m, m.scheduleDetail()
}
