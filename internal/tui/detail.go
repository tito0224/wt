package tui

import (
	"errors"
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
	return m, loadDetailCmd(m.detailGen, wt.Path, wt.Branch, wt.BaseRef)
}

// detailLogCount is how many recent commits to load for the detail screen. The
// status bar uses only the first (HEAD).
const detailLogCount = 5

// loadDetailCmd fetches the recent commit log and (best-effort) the PR for a
// worktree off the main loop. The forge lookup is cached in the forge package,
// so re-selecting a worktree in a later session's first hit is the only slow one.
func loadDetailCmd(gen int, path, branch, baseRef string) tea.Cmd {
	return func() tea.Msg {
		var d detail
		if commits, err := git.RecentCommits(path, detailLogCount); err != nil {
			d.commitErr = err
		} else {
			d.commits = commits
		}
		if st, err := git.Status(path); err != nil {
			d.statusErr = err
		} else {
			d.status = st
		}
		// A PR, a definitive "no PR" (ErrNoPR), or a transient failure. Only
		// mark prChecked on a definitive answer so a failed lookup can retry.
		if pr, err := forge.LookupPR(path, branch); err == nil {
			d.prChecked = true
			d.pr = pr
			if pr.State == "MERGED" && pr.MergedAt != "" {
				// Detect commits pushed after the merge (real unmerged work).
				// Uses commit dates because squash/rebase merges rewrite SHAs,
				// so reachability alone can't tell them apart.
				d.postMerge, _ = git.CommitsAfter(path, baseRef, pr.MergedAt)
			}
		} else if errors.Is(err, forge.ErrNoPR) {
			d.prChecked = true // definitively no PR
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
