package tui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/johnlotito/wt/internal/forge"
	"github.com/johnlotito/wt/internal/git"
)

// detailDebounce is how long to wait after the selection settles before
// loading detail, so rapid j/k scrolling doesn't spawn a load (and a gh call)
// per intermediate row.
const detailDebounce = 150 * time.Millisecond

// scheduleDetail is called whenever the selection changes. If the highlighted
// worktree's detail is already cached it does nothing; otherwise it advances
// the generation and returns a debounce timer command. The eventual
// detailDebounceMsg is ignored if the generation has since advanced.
func (m *model) scheduleDetail() tea.Cmd {
	wt := m.selected()
	if wt == nil {
		return nil
	}
	if _, ok := m.details[wt.Path]; ok {
		return nil // already loaded
	}
	m.detailGen++
	gen := m.detailGen
	return tea.Tick(detailDebounce, func(time.Time) tea.Msg {
		return detailDebounceMsg{gen: gen}
	})
}

// handleDetailDebounce fires after the debounce window. If still current and
// uncached, it kicks off the async detail load for the selected worktree.
func (m *model) handleDetailDebounce(msg detailDebounceMsg) (tea.Model, tea.Cmd) {
	if msg.gen != m.detailGen {
		return m, nil // selection moved on
	}
	wt := m.selected()
	if wt == nil {
		return m, nil
	}
	if _, ok := m.details[wt.Path]; ok {
		return m, nil
	}
	return m, loadDetailCmd(m.detailGen, wt.Path, wt.Branch)
}

// loadDetailCmd fetches the last commit and (best-effort) the PR for a
// worktree off the main loop. The forge lookup is cached in the forge package,
// so re-selecting a worktree in a later session's first hit is the only slow one.
func loadDetailCmd(gen int, path, branch string) tea.Cmd {
	return func() tea.Msg {
		var d detail
		if c, err := git.LastCommit(path); err != nil {
			d.commitErr = err
		} else {
			d.commit = c
		}
		if pr, found := forge.LookupPR(path, branch); found {
			d.pr = pr
			d.prChecked = true
		}
		return detailResultMsg{gen: gen, path: path, detail: d}
	}
}

// handleDetailResult stores a loaded detail, ignoring stale results.
func (m *model) handleDetailResult(msg detailResultMsg) (tea.Model, tea.Cmd) {
	// Cache by path regardless of generation (the data is still valid), but
	// only the current selection's result affects what's on screen.
	m.details[msg.path] = msg.detail
	return m, nil
}
