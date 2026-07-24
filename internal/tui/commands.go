package tui

import (
	"fmt"
	"os/exec"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/johnlotito/wt/internal/git"
	"github.com/johnlotito/wt/internal/ops"
	"github.com/johnlotito/wt/internal/terminal"
)

// execDoneMsg is delivered after a suspended external process (diff/difftool)
// returns and the TUI has resumed.
type execDoneMsg struct{ err error }

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

// runAction dispatches a keybar action for the selected worktree. "cd" quits
// and hands the path back to Run; "open" launches a terminal tab; "openpr"
// opens the PR in a browser; "remove" opens the confirmation prompt.
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
			return m, nil
		}
		if err := adapter.OpenTab(wt.Path); err != nil {
			m.setErr(err.Error())
			return m, nil
		}
		m.setMsg(fmt.Sprintf("opened %s in new %s tab", wt.Label(), adapter.Name()))
	case "openpr":
		pr := m.selectedPR()
		if pr == nil {
			m.setErr("no pull request for this worktree")
			return m, nil
		}
		if err := openURL(pr.URL); err != nil {
			m.setErr(err.Error())
			return m, nil
		}
		m.setMsg(fmt.Sprintf("opened PR #%d in browser", pr.Number))
	case "diff", "difftool":
		// git diff/difftool are interactive (pager or GUI tool). Suspend the
		// TUI, run in the worktree, and resume when it exits.
		if wt.Dirty {
			sub := "diff"
			if id == "difftool" {
				sub = "difftool"
			}
			c := exec.Command("git", "-C", wt.Path, sub)
			return m, tea.ExecProcess(c, func(err error) tea.Msg {
				return execDoneMsg{err: err}
			})
		}
		m.setMsg("no uncommitted changes to diff")
		return m, nil
	case "cd":
		m.resultAction = id
		m.resultWT = wt
		return m, tea.Quit
	case "remove":
		// Ask for confirmation before a destructive removal (especially since
		// dirty worktrees are force-removed).
		m.list.confirm = "remove"
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
		branch, path, err := ops.AddWorktree(top, name, "", "")
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

// doRemove kicks off asynchronous worktree removal after confirmation. When
// deleteBranch is set, it also deletes the branch (forcing when forceBranch is
// set). The blocking git work runs in a tea.Cmd so the spinner can animate;
// completion arrives as removeResultMsg.
func (m *model) doRemove(deleteBranch, forceBranch bool) (tea.Model, tea.Cmd) {
	wt := m.selected()
	if wt == nil {
		return m, nil
	}
	// Pre-flight the guards synchronously so we can report them without a
	// spinner round-trip. (ops.RemoveWorktree re-checks them; this just gives a
	// fast, inline error.)
	if wt.Current {
		m.setErr(ops.ErrCurrent.Error())
		return m, nil
	}

	label := wt.Label()
	path := wt.Path
	target := *wt
	dir := m.dir
	opts := ops.RemoveOptions{Force: wt.Dirty, DeleteBranch: deleteBranch, ForceBranch: forceBranch}

	cmd := func() tea.Msg {
		res, err := ops.RemoveWorktree(dir, target, opts)
		return removeResultMsg{label: label, path: path, force: opts.Force, result: res, err: err}
	}
	return m, tea.Batch(m.startBusy(fmt.Sprintf("removing %s…", label)), cmd)
}

// handleRemoveResult applies the outcome of an async removal. On success it
// drops the worktree from the list. If the worktree was removed but a requested
// branch deletion was refused (unmerged), it prompts to force-delete.
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
	// Removal always returns to the list (e.g. when triggered from the detail
	// screen, that worktree is gone, so its detail view no longer makes sense).
	m.view = viewList

	switch {
	case msg.result.BranchUnmerged:
		// Worktree gone, but the (safe) branch delete was refused because the
		// branch is unmerged. Offer to force it.
		m.pendingBranch = msg.result.Branch
		m.list.confirm = "force-branch"
		m.setMsg(fmt.Sprintf("removed %s%s; branch %s is unmerged", msg.label, note, msg.result.Branch))
		return m, nil
	case msg.result.BranchDeleted:
		m.setMsg(fmt.Sprintf("removed %s%s and deleted branch %s", msg.label, note, msg.result.Branch))
	default:
		m.setMsg(fmt.Sprintf("removed %s%s", msg.label, note))
	}
	// The cursor now points at a different worktree; load its detail.
	return m, m.scheduleDetail()
}

// doForceDeleteBranch force-deletes the branch left over after a removal whose
// safe branch delete was refused.
func (m *model) doForceDeleteBranch() (tea.Model, tea.Cmd) {
	branch := m.pendingBranch
	m.pendingBranch = ""
	if branch == "" {
		return m, nil
	}
	dir := m.dir
	cmd := func() tea.Msg {
		err := ops.DeleteBranch(dir, branch, true)
		return branchDeleteResultMsg{branch: branch, err: err}
	}
	return m, tea.Batch(m.startBusy(fmt.Sprintf("deleting branch %s…", branch)), cmd)
}

// handleBranchDeleteResult reports the outcome of a forced branch deletion.
func (m *model) handleBranchDeleteResult(msg branchDeleteResultMsg) (tea.Model, tea.Cmd) {
	m.spinner.active = false
	m.spinner.msg = ""
	if msg.err != nil {
		m.setErr(fmt.Sprintf("branch %s not deleted: %v", msg.branch, msg.err))
	} else {
		m.setMsg(fmt.Sprintf("deleted branch %s", msg.branch))
	}
	return m, nil
}
