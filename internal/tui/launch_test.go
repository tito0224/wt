package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/johnlotito/wt/internal/launcher"
)

// fakeLauncher is a no-op GUI launcher so tests can drive the launch submenu
// without probing the host for installed editors (launcher.Available shells
// out to `command -v` style checks and is environment-dependent).
type fakeLauncher struct{}

func (fakeLauncher) Name() string            { return "fake" }
func (fakeLauncher) Title() string           { return "Fake Editor" }
func (fakeLauncher) Icon() string            { return "" }
func (fakeLauncher) Kind() launcher.Kind     { return launcher.KindGUI }
func (fakeLauncher) Available() bool         { return true }
func (fakeLauncher) Command(string) []string { return nil }
func (fakeLauncher) GUILaunch(string) error  { return nil }
func (fakeLauncher) URL(string) string       { return "" }

// TestLaunchReturnsToOriginView is a regression test: e/E from the DETAIL
// screen used to dump the user back on the LIST view because the launch flow
// hard-coded viewList on exit. Both esc and a completed launch must return to
// wherever the flow was entered from.
func TestLaunchReturnsToOriginView(t *testing.T) {
	initStyles(lipgloss.DefaultRenderer())
	items := []launcher.Launcher{fakeLauncher{}}

	// Entered from the detail screen: esc returns to detail.
	m := &model{width: 80, height: 24, rows: rowsFixture(), view: viewDetail}
	m.enterLaunchWith(items)
	if m.view != viewLaunch {
		t.Fatalf("view=%v, want viewLaunch after enterLaunch", m.view)
	}
	m.updateLaunch(tea.KeyMsg{Type: tea.KeyEsc})
	if m.view != viewDetail {
		t.Fatalf("view=%v, want viewDetail after esc from launch entered on detail", m.view)
	}

	// Launching (enter) also returns to the origin view — launching never
	// removes the worktree, so the detail screen is still valid.
	m.enterLaunchWith(items)
	m.updateLaunch(tea.KeyMsg{Type: tea.KeyEnter})
	if m.view != viewDetail {
		t.Fatalf("view=%v, want viewDetail after launching from detail", m.view)
	}

	// Entered from the list: q returns to the list.
	m.view = viewList
	m.enterLaunchWith(items)
	m.updateLaunch(runeKey('q'))
	if m.view != viewList {
		t.Fatalf("view=%v, want viewList after q from launch entered on list", m.view)
	}
}
