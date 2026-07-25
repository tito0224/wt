// Package tui provides an interactive worktree picker rendered as a scrollable,
// fuzzy-filterable table, with a per-worktree status bar and a full details
// screen.
//
// The code is organized by concern:
//
//   - tui.go            the model, message types, Run, and the Update/View dispatch
//   - styles.go         lipgloss styles
//   - spinner.go        the busy/loading spinner
//   - statusbar.go      the selection status bar + key bar
//   - detail.go         async loading of per-worktree detail (commits, PR)
//   - commands.go       async git effects and their result reducers
//   - render.go         shared rendering + layout helpers
//   - view_*.go         per-view update + render (list, detail, create, loading)
package tui

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/tito0224/wt/internal/config"
	"github.com/tito0224/wt/internal/forge"
	"github.com/tito0224/wt/internal/git"
	"github.com/tito0224/wt/internal/launcher"
	"github.com/tito0224/wt/internal/ops"
)

type view int

const (
	viewList view = iota
	viewDetail
	viewCreate
	viewMove
	viewHelp
	viewLaunch
)

// loadResultMsg is delivered when the initial (or a refreshed) worktree
// enumeration finishes.
type loadResultMsg struct {
	wts []git.Worktree
	err error
}

// branchListMsg carries the local branches loaded for the add screen.
type branchListMsg struct {
	branches []git.Branch
	err      error
}

// createResultMsg is delivered when an async worktree creation finishes. On
// success it carries the refreshed worktree set and the new worktree's path.
type createResultMsg struct {
	branch string
	path   string
	wts    []git.Worktree
	err    error
}

// moveResultMsg is delivered when an async worktree move finishes. On success
// it carries the refreshed worktree set and the new worktree path.
type moveResultMsg struct {
	path string
	wts  []git.Worktree
	err  error
}

// removeResultMsg is delivered when an async worktree removal finishes. result
// carries the typed outcome (branch deleted / left unmerged); err is the
// removal error (nil on success).
type removeResultMsg struct {
	label  string
	path   string
	force  bool
	result ops.RemoveResult
	err    error
}

// branchDeleteResultMsg is delivered when a standalone (forced) branch deletion
// finishes — used for the force-delete follow-up after an unmerged refusal.
type branchDeleteResultMsg struct {
	branch string
	err    error
}

// detailDebounceMsg fires after a short delay following a selection change. It
// carries the generation it was scheduled for; if the selection has moved on
// (gen advanced) it is ignored, so rapid j/k scrolling only loads the row the
// user lands on.
type detailDebounceMsg struct {
	gen int
}

// detailResultMsg carries the loaded detail for a worktree. path identifies
// which worktree it belongs to (the cursor may have moved); gen guards against
// stale results.
type detailResultMsg struct {
	gen    int
	path   string
	detail detail
}

// detail holds the extra, lazily-loaded information shown for the highlighted
// worktree: a one-line summary in the status bar (last commit + PR) and, on the
// detail screen, the working-tree status and recent commit log.
type detail struct {
	commits   []git.Commit // most-recent first; commits[0] is HEAD
	commitErr error

	status    git.WorktreeStatus
	statusErr error

	pr        *forge.PR
	prChecked bool // true once a forge lookup completed (found or definitively none)
	// postMerge counts commits on the branch made after the PR merged (only
	// meaningful when pr.State == MERGED); >0 means genuinely-unmerged new work.
	postMerge int
}

// last returns the HEAD commit, or a zero Commit when the log is empty.
func (d detail) last() git.Commit {
	if len(d.commits) == 0 {
		return git.Commit{}
	}
	return d.commits[0]
}

// row is a worktree paired with its fuzzy-match metadata for the current
// filter query.
type row struct {
	wt      git.Worktree
	score   int   // fuzzy-match score; only meaningful when a filter is active
	matched []int // matched rune indices in the label, for highlighting
}

// listState is the list screen's own state: the fuzzy filter, whether the
// search input is active, and any pending destructive-action confirmation
// ("remove" for the initial y/b/n prompt, "force-branch" for the follow-up
// unmerged-branch force-delete prompt).
type listState struct {
	filter    string
	searching bool   // true while in search-input mode (entered with '/')
	confirm   string // non-empty shows a confirmation prompt for this action
}

// createRow is one selectable line in the add screen. It is either the
// "create a new branch" option (New == true, using the typed/generated name) or
// an existing branch to check out.
type createRow struct {
	New     bool       // the top "new branch" option
	branch  git.Branch // the existing branch (when !New)
	matched []int      // fuzzy-match highlight indices into the display text
}

// selectable reports whether the row can be chosen. The new-branch option is
// always selectable; a branch row is selectable unless it's already in use.
func (r createRow) selectable() bool { return r.New || !r.branch.InUse() }

// moveState is the move screen's state: the text input for the destination
// path and the worktree being moved.
type moveState struct {
	input textinput.Model
	wt    git.Worktree
}

// createState is the add screen's state: the text input (a bubbles textinput,
// which handles cursor movement/editing), the list of existing local branches
// (loaded async), and the filtered/selectable rows. Row 0 is always the "new
// branch" option; the rest are matching branches.
type createState struct {
	input       textinput.Model
	branches    []git.Branch // all local branches; loaded once on entering the view
	loaded      bool
	rows        []createRow // filtered view: row 0 = new, then matching branches
	selectedRow int         // index into rows

	// top and cfg are resolved once in enterCreate (a git subprocess + config
	// file read). The render path uses them to preview the new branch name and
	// worktree path and repaints every cursor blink (~2×/sec), so it must not
	// shell out per frame.
	top string        // repo top-level dir ("" when resolution failed)
	cfg config.Config // create defaults (branch prefix, worktree dir template)
}

// launchState is the launcher submenu's state: the available launchers for the
// worktree being opened, and the highlighted choice.
type launchState struct {
	items       []launcher.Launcher
	selectedRow int
	wtPath      string // the worktree the menu was opened for
	wtLabel     string
}

// spinnerState is the cross-cutting busy/loading indicator state. active is
// true during an async git operation (create/remove); loading is true during
// the initial worktree enumeration. Either animates the spinner; msg labels an
// active operation. model is the bubbles spinner that owns the animation.
type spinnerState struct {
	active  bool
	loading bool
	msg     string
	model   spinner.Model
}

type model struct {
	dir             string
	all             []git.Worktree // full, unfiltered set
	terminalName    string
	defaultLauncher string // configured quick-launch editor/agent (may be empty)

	view         view
	helpReturn   view  // view to restore when leaving the help overlay
	launchReturn view  // view to restore when leaving the launcher submenu
	rows         []row // filtered view of `all`

	list    listState
	move    moveState
	create  createState
	spinner spinnerState
	launch  launchState

	// pendingBranch is the branch awaiting a force-delete decision after a
	// worktree removal whose safe branch delete was refused (unmerged).
	pendingBranch string

	// details caches loaded status-bar info per worktree path. detailGen is
	// bumped on every selection change to debounce and discard stale async
	// loads (see detailDebounceMsg/detailResultMsg).
	details   map[string]detail
	detailGen int

	// pendingSelect is a worktree path to re-select once the next load
	// completes. Set by the refresh action (R) so the cursor doesn't jump to
	// the top when the reloaded set arrives; consumed in handleLoadResult.
	pendingSelect string
	// pendingMsg is a success message to show once the next load completes.
	// Actions that end in a reload (e.g. prune) park their result message
	// here because startBusy clears the live one; consumed in
	// handleLoadResult.
	pendingMsg string

	selectedRow int // index of the highlighted worktree row
	offset      int // scroll offset (first visible row)

	width, height int

	message string
	isErr   bool
	quit    bool

	// resultAction/resultWT carry the chosen action and worktree back to Run
	// after the program exits. resultWT is a copy (valid when resultOK), never
	// a pointer into m.rows — see selected() for why aliasing rows is unsafe.
	resultAction string
	resultWT     git.Worktree
	resultOK     bool
}

// Run launches the interactive picker.
//
// The TUI renders to stderr and reads from stdin — the standard pattern for a
// tool invoked as $(wt) by the shell wrapper. Command substitution captures
// only stdout, so drawing to stderr keeps the UI visible while leaving stdout
// free for the single line of result (the chosen worktree path, for `cd`).
// Using the process's real stdin/stderr (rather than opening /dev/tty) lets the
// Bubble Tea and Bubbles components work as designed and keeps things portable.
func Run(dir, terminalName, defaultLauncher string) error {
	// Start empty and in the loading state; Init kicks off async enumeration.
	m := &model{
		dir:             dir,
		terminalName:    terminalName,
		defaultLauncher: defaultLauncher,
		width:           80,
		height:          24,
		spinner:         spinnerState{loading: true},
		details:         map[string]detail{},
	}
	m.recompute()

	// Render to stderr; detect color from stderr (stdout may be captured by the
	// shell wrapper's $(...), which would strip color).
	initStyles(lipgloss.NewRenderer(os.Stderr))
	m.spinner.model = newSpinner() // after initStyles: uses titleStyle

	// Bubble Tea's built-in panic handling (enabled by default) recovers panics
	// from both the update loop and command goroutines, restores the terminal,
	// and prints the panic value + stack trace before the program exits. We rely
	// on it rather than rolling our own so a panic anywhere crashes the app
	// visibly instead of being swallowed.
	p := tea.NewProgram(m,
		tea.WithAltScreen(),
		tea.WithOutput(os.Stderr),
	)

	res, err := p.Run()
	if err != nil {
		return err
	}
	fm := res.(*model)
	if !fm.resultOK {
		return nil
	}
	if fm.resultAction == "cd" {
		fmt.Println(fm.resultWT.Path)
	}
	return nil
}

func (m *model) Init() tea.Cmd {
	// Begin animating the spinner and enumerating worktrees concurrently.
	return tea.Batch(m.spinner.model.Tick, loadCmd(m.dir))
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.clampScroll()
		// Force a full repaint so shrinking the terminal can't leave stale
		// lines from the previous (taller/wider) frame on screen.
		return m, tea.ClearScreen
	case spinner.TickMsg:
		// Advance the spinner only while some git operation is still in flight;
		// otherwise let the tick loop stop.
		if !m.spinner.active && !m.spinner.loading {
			return m, nil
		}
		var cmd tea.Cmd
		m.spinner.model, cmd = m.spinner.model.Update(msg)
		return m, cmd
	case loadResultMsg:
		return m.handleLoadResult(msg)
	case branchListMsg:
		return m.handleBranchList(msg)
	case createResultMsg:
		return m.handleCreateResult(msg)
	case moveResultMsg:
		return m.handleMoveResult(msg)
	case removeResultMsg:
		return m.handleRemoveResult(msg)
	case branchDeleteResultMsg:
		return m.handleBranchDeleteResult(msg)
	case detailDebounceMsg:
		return m.handleDetailDebounce(msg)
	case detailResultMsg:
		return m.handleDetailResult(msg)
	case execDoneMsg:
		if msg.err != nil {
			m.setErr(msg.err.Error())
		}
		// The external tool may have changed the working tree (difftool can
		// stage/discard changes), so the cached status counts are stale; drop
		// the selection's detail and reload it.
		if p := m.selectedPath(); p != "" {
			delete(m.details, p)
		}
		// Force a repaint: the external program scribbled over the alt-screen.
		return m, tea.Batch(tea.ClearScreen, m.scheduleDetail())
	case tea.KeyMsg:
		// While loading or a git operation is running, swallow input except
		// Ctrl-C so the spinner isn't interrupted and no conflicting action
		// starts.
		if m.spinner.active || m.spinner.loading {
			if msg.Type == tea.KeyCtrlC {
				m.quit = true
				return m, tea.Quit
			}
			return m, nil
		}
		if m.list.confirm != "" {
			return m.updateConfirm(msg)
		}
		if m.view == viewHelp {
			return m.updateHelp(msg)
		}
		if m.view == viewCreate {
			return m.updateCreate(msg)
		}
		if m.view == viewMove {
			return m.updateMove(msg)
		}
		if m.view == viewDetail {
			return m.updateDetail(msg)
		}
		if m.view == viewLaunch {
			return m.updateLaunch(msg)
		}
		// Track the selected worktree across the key handler so a changed
		// selection can (debounced) load its status-bar detail.
		before := m.selectedPath()
		newM, cmd := m.updateList(msg)
		if mm, ok := newM.(*model); ok && mm.selectedPath() != before {
			return mm, tea.Batch(cmd, mm.scheduleDetail())
		}
		return newM, cmd
	}
	// Non-key messages (e.g. the textinput's cursor blink) are forwarded to the
	// add or move screen's input while active, so the cursor animates.
	if m.view == viewCreate {
		var cmd tea.Cmd
		m.create.input, cmd = m.create.input.Update(msg)
		return m, cmd
	}
	if m.view == viewMove {
		var cmd tea.Cmd
		m.move.input, cmd = m.move.input.Update(msg)
		return m, cmd
	}
	return m, nil
}

// selectedPath returns the highlighted worktree's path, or "" when none.
func (m *model) selectedPath() string {
	if wt, ok := m.selected(); ok {
		return wt.Path
	}
	return ""
}

// selectedPR returns the loaded PR for the highlighted worktree, or nil when
// none is known (not loaded, no forge, or no PR).
func (m *model) selectedPR() *forge.PR {
	wt, ok := m.selected()
	if !ok {
		return nil
	}
	if d, ok := m.details[wt.Path]; ok {
		return d.pr
	}
	return nil
}

// selectedDirty reports whether the highlighted worktree has uncommitted
// changes (so diff actions apply).
func (m *model) selectedDirty() bool {
	wt, ok := m.selected()
	return ok && wt.Dirty
}

func (m *model) View() string {
	if m.quit {
		return ""
	}
	if m.spinner.loading {
		return m.clampFrame(m.viewLoadingRender())
	}
	var raw string
	switch m.view {
	case viewDetail:
		raw = m.viewDetailRender()
	case viewMove:
		raw = m.viewMoveRender()
	case viewCreate:
		raw = m.viewCreateRender()
	case viewHelp:
		raw = m.viewHelpRender()
	case viewLaunch:
		raw = m.viewLaunchRender()
	default:
		raw = m.viewListRender()
	}
	return m.clampFrame(raw)
}

// recompute rebuilds the filtered rows from the current filter. Rows keep
// git's native order (repo root first, as git lists it). When a filter is
// active, matches are sorted by fuzzy score (best match first).
func (m *model) recompute() {
	q := strings.ToLower(strings.TrimSpace(m.list.filter))
	m.rows = m.rows[:0]
	for _, w := range m.all {
		if q == "" {
			m.rows = append(m.rows, row{wt: w})
			continue
		}
		// Match against "branch  path" so either field can satisfy the query.
		//
		// NOTE: match indices are computed against the lowercased haystack but
		// applied to the original-case label below. This assumes lowercasing
		// is rune-count-stable (true for ASCII and nearly all of Unicode);
		// exotic pairs like 'İ' (which lowercases to two runes) would skew the
		// highlight positions by a rune — cosmetic only, so we accept it.
		hay := strings.ToLower(w.Label() + " " + w.Path)
		if idx, score, ok := fuzzyMatch(hay, q); ok {
			// Only keep highlight indices that fall within the label portion.
			var inLabel []int
			labelLen := len([]rune(w.Label()))
			for _, i := range idx {
				if i < labelLen {
					inLabel = append(inLabel, i)
				}
			}
			m.rows = append(m.rows, row{wt: w, score: score, matched: inLabel})
		}
	}
	// Rank matches best-first only while filtering; the unfiltered list keeps
	// git's native order. Stable sort preserves git order among equal scores.
	if q != "" {
		sort.SliceStable(m.rows, func(i, j int) bool {
			return m.rows[i].score > m.rows[j].score
		})
	}
	if m.selectedRow >= len(m.rows) {
		m.selectedRow = len(m.rows) - 1
	}
	if m.selectedRow < 0 {
		m.selectedRow = 0
	}
	m.clampScroll()
}

// visibleRows is how many table rows fit given the terminal height, reserving
// space for the surrounding chrome:
//
//	title(1) + filter(1) + header(1) + scroll(1)
//	+ statusbar(blank + up to 3 lines = 4) + footer(blank + keybar = 2)
//
// which is 10 lines. When the terminal is very short, at least one row is shown.
func (m *model) visibleRows() int {
	chrome := 10
	// The prune banner occupies one extra line when stale entries exist.
	if m.prunableCount() > 0 {
		chrome++
	}
	n := m.height - chrome
	if n < 1 {
		n = 1
	}
	return n
}

func (m *model) clampScroll() {
	vis := m.visibleRows()
	if m.selectedRow < m.offset {
		m.offset = m.selectedRow
	}
	if m.selectedRow >= m.offset+vis {
		m.offset = m.selectedRow - vis + 1
	}
	if m.offset < 0 {
		m.offset = 0
	}
}

// selected returns a copy of the highlighted worktree (ok=false when the list
// is empty). It deliberately returns a value, not a pointer into m.rows:
// recompute() reuses the rows backing array (m.rows = m.rows[:0]), so a
// retained pointer would silently alias whatever row lands in that slot after
// the next refilter/reload.
func (m *model) selected() (git.Worktree, bool) {
	if m.selectedRow < 0 || m.selectedRow >= len(m.rows) {
		return git.Worktree{}, false
	}
	return m.rows[m.selectedRow].wt, true
}

func (m *model) moveCursor(delta int) {
	m.message = ""
	m.selectedRow += delta
	if m.selectedRow < 0 {
		m.selectedRow = 0
	}
	if m.selectedRow > len(m.rows)-1 {
		m.selectedRow = len(m.rows) - 1
	}
	m.clampScroll()
}

// selectByPath moves the cursor to the row whose worktree path matches, if
// present, scrolling it into view.
func (m *model) selectByPath(path string) {
	for i := range m.rows {
		if m.rows[i].wt.Path == path {
			m.selectedRow = i
			m.clampScroll()
			return
		}
	}
}

func (m *model) removeFromAll(path string) {
	for i := range m.all {
		if m.all[i].Path == path {
			m.all = append(m.all[:i], m.all[i+1:]...)
			break
		}
	}
	m.recompute()
}

func (m *model) setMsg(s string) { m.message, m.isErr = s, false }
func (m *model) setErr(s string) { m.message, m.isErr = s, true }
