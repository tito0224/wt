package tui

import (
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/tito0224/wt/internal/git"
)

func rowsFixture() []row {
	wts := []git.Worktree{
		{Path: "/home/u/projects/main", Branch: "main", Current: true, Dirty: false},
		{Path: "/home/u/projects/a-really-quite-long-feature-branch-name-here", Branch: "a-really-quite-long-feature-branch-name-here", Ahead: 2, Behind: 1, Dirty: true, HasUpstream: true},
		{Path: "/home/u/projects/fix", Branch: "fix", AheadOfBase: 3, BaseBranch: "main"},
	}
	rs := make([]row, len(wts))
	for i, w := range wts {
		rs[i] = row{wt: w}
	}
	return rs
}

func TestLayoutNoOverflow(t *testing.T) {
	initStyles(lipgloss.DefaultRenderer())
	for _, width := range []int{200, 120, 80, 60, 40, 30, 20, 10} {
		m := &model{width: width, height: 24, rows: rowsFixture()}
		lay := m.computeLayout()
		for i, r := range m.rows {
			for _, selected := range []bool{false, true} {
				line := m.renderRow(r, selected, lay)
				w := lipgloss.Width(line)
				if w > width {
					t.Errorf("width=%d row=%d selected=%v: rendered width %d exceeds terminal width\n  line=%q",
						width, i, selected, w, line)
				}
			}
		}
		t.Logf("width=%3d -> branchW=%d statusW=%d pathW=%d showPath=%v", width, lay.branchW, lay.statusW, lay.pathW, lay.showPath)
	}
}
