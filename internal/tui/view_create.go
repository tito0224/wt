package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/tito0224/wt/internal/config"
	"github.com/tito0224/wt/internal/git"
)

// enterCreate initializes and opens the add screen, kicking off the async
// branch load.
func (m *model) enterCreate() (tea.Model, tea.Cmd) {
	m.view = viewCreate
	m.message = ""

	in := textinput.New()
	in.Prompt = "" // the surrounding view provides framing
	in.Placeholder = "new branch name"
	focusCmd := in.Focus() // returns the cursor-blink command

	// Resolve the repo root and config here, once, on entry — the render path
	// needs them for the new-branch preview but repaints on every cursor blink,
	// so it must not run a git subprocess or re-read config files per frame.
	// A single quick synchronous git call on entry is acceptable. Failures
	// leave top empty; the preview then simply shows less detail.
	top := ""
	if m.dir != "" {
		if t, err := git.TopLevel(m.dir); err == nil {
			top = t
		}
	}
	var cfg config.Config
	if top != "" {
		cfg, _ = config.Load(top)
	}
	m.create = createState{input: in, top: top, cfg: cfg}

	m.recomputeCreate()
	return m, tea.Batch(loadBranchesCmd(m.dir), focusCmd)
}

// updateCreate handles keys on the add screen: up/down move the branch-list
// selection; everything else (typing, cursor movement, editing) is delegated
// to the textinput. Enter acts on the selected row; esc cancels.
func (m *model) updateCreate(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.Type {
	case tea.KeyCtrlC:
		m.quit = true
		return m, tea.Quit
	case tea.KeyEsc:
		m.view = viewList
		m.create = createState{}
		return m, nil
	case tea.KeyEnter:
		return m.doCreateSelected()
	case tea.KeyUp:
		m.moveCreateCursor(-1)
		return m, nil
	case tea.KeyDown:
		m.moveCreateCursor(1)
		return m, nil
	}

	// Delegate all other keys (runes, backspace, delete, left/right, home/end,
	// word jumps, paste) to the textinput. Re-filter when the value changes.
	before := m.create.input.Value()
	var cmd tea.Cmd
	m.create.input, cmd = m.create.input.Update(key)
	if m.create.input.Value() != before {
		m.recomputeCreate()
	}
	return m, cmd
}

// moveCreateCursor moves the selection within the create rows, skipping
// non-selectable (in-use) branch rows, and clamps at the ends.
func (m *model) moveCreateCursor(delta int) {
	n := len(m.create.rows)
	if n == 0 {
		return
	}
	step := 1
	if delta < 0 {
		step = -1
	}
	i := m.create.selectedRow
	for c := 0; c < abs(delta); c++ {
		next := i
		// Advance to the next selectable row in the step direction.
		for {
			next += step
			if next < 0 || next >= n {
				next = i // no selectable row that way; stay put
				break
			}
			if m.create.rows[next].selectable() {
				break
			}
		}
		i = next
	}
	m.create.selectedRow = i
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// recomputeCreate rebuilds the filtered rows from the current query. Row 0 is
// always the "new branch" option; matching existing branches follow. The
// selection is kept on row 0 (the new-branch option) whenever the query
// changes, per the desired behavior.
func (m *model) recomputeCreate() {
	q := strings.ToLower(strings.TrimSpace(m.create.input.Value()))
	rows := []createRow{{New: true}}
	for _, br := range m.create.branches {
		if q == "" {
			rows = append(rows, createRow{branch: br})
			continue
		}
		if idx, _, ok := fuzzyMatch(strings.ToLower(br.Name), q); ok {
			rows = append(rows, createRow{branch: br, matched: idx})
		}
	}
	m.create.rows = rows
	// Keep the create-new option selected as the query changes; the user can
	// arrow down into the branch list explicitly.
	m.create.selectedRow = 0
}

// doCreateSelected acts on the highlighted create row: the new-branch option
// creates a worktree with the generated name; a branch row adds a worktree for
// that existing branch (unless it is already in use).
func (m *model) doCreateSelected() (tea.Model, tea.Cmd) {
	if m.create.selectedRow < 0 || m.create.selectedRow >= len(m.create.rows) {
		return m, nil
	}
	row := m.create.rows[m.create.selectedRow]
	if row.New {
		return m.doCreate()
	}
	if row.branch.InUse() {
		m.setErr(fmt.Sprintf("branch %s is already checked out", row.branch.Name))
		return m, nil
	}
	return m.doAddExisting(row.branch.Name)
}

func (m *model) viewCreateRender() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("add worktree") + "\n\n")
	b.WriteString(helpStyle.Render("name a new branch, or pick an existing one") + "\n")
	b.WriteString(m.create.input.View() + "\n\n")

	// Show a window of rows around the cursor so a long branch list can't push
	// the footer off screen. Reserve ~8 lines for title/help/input/footer.
	vis := m.height - 8
	if vis < 3 {
		vis = 3
	}
	start := 0
	if m.create.selectedRow >= vis {
		start = m.create.selectedRow - vis + 1
	}
	end := start + vis
	if end > len(m.create.rows) {
		end = len(m.create.rows)
	}
	if start > 0 {
		b.WriteString(pathStyle.Render("  ↑ more") + "\n")
	}
	for i := start; i < end; i++ {
		selected := i == m.create.selectedRow
		b.WriteString(m.renderCreateRow(m.create.rows[i], selected) + "\n")
	}
	if end < len(m.create.rows) {
		b.WriteString(pathStyle.Render("  ↓ more") + "\n")
	}

	if !m.create.loaded {
		b.WriteString(pathStyle.Render("  loading branches…") + "\n")
	}

	b.WriteString("\n" + m.footer("↑/↓ select · enter add · esc cancel"))
	return b.String()
}

// renderCreateRow renders one row of the add screen. Selected rows are drawn in
// reverse video with plain (uncolored) text so they stay readable on any theme,
// mirroring the worktree list. In-use branches are dimmed and never selectable.
// The branch/path preview uses the top/cfg resolved in enterCreate — never
// re-resolved here (this runs per frame).
func (m *model) renderCreateRow(row createRow, selected bool) string {
	// Build the plain (unstyled) text and, separately, the styled non-selected
	// rendering.
	if row.New {
		name := strings.TrimSpace(m.create.input.Value())
		var plain, styled string
		if name == "" {
			plain = "＋ new branch (type a name)"
			styled = helpStyle.Render(plain)
		} else {
			branch := m.create.cfg.Create.BranchName(name)
			path := abbrevHome(m.create.cfg.Create.WorktreeDir(m.create.top, name))
			plain = "＋ new branch " + branch + "  " + path
			styled = cleanStyle.Render("＋ new branch "+branch) + pathStyle.Render("  "+path)
		}
		if selected {
			return selectedStyle.Render("> " + plain)
		}
		return "  " + styled
	}

	br := row.branch
	if br.InUse() {
		// Dimmed and non-selectable regardless of cursor position.
		return "  " + pathStyle.Render(br.Name+"  (in use)")
	}
	if selected {
		return selectedStyle.Render("> " + br.Name)
	}
	return "  " + highlight(br.Name, row.matched)
}
