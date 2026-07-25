package tui

import (
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/tito0224/wt/internal/git"
)

// TestFilterShadowedRemotes pins the add-screen listing policy: remote
// branches with a local counterpart are hidden (the local row already covers
// them); everything else passes through in order.
func TestFilterShadowedRemotes(t *testing.T) {
	in := []git.Branch{
		{Name: "main", CheckedOutAt: "/r"},
		{Name: "feat"},
		{Name: "main", Remote: "origin"},        // shadowed by local main
		{Name: "remote-only", Remote: "origin"}, // no local counterpart
		{Name: "feat", Remote: "upstream"},      // shadowed by local feat
	}
	out := filterShadowedRemotes(in)

	want := []string{"main", "feat", "origin/remote-only"}
	if len(out) != len(want) {
		t.Fatalf("got %d branches, want %d: %+v", len(out), len(want), out)
	}
	for i, w := range want {
		if out[i].FullName() != w {
			t.Errorf("branch %d = %q, want %q", i, out[i].FullName(), w)
		}
	}
}

// TestCreateRowsIncludeRemote verifies remote branches appear as selectable
// rows and are matchable by their full "remote/name" display name.
func TestCreateRowsIncludeRemote(t *testing.T) {
	initStyles(lipgloss.DefaultRenderer())
	m := &model{width: 80, height: 24}
	newM, _ := m.enterCreate()
	m = newM.(*model)
	m.create.branches = []git.Branch{
		{Name: "local-a"},
		{Name: "remote-only", Remote: "origin"},
	}
	m.recomputeCreate()

	// Row 0 is the new-branch option; the two branches follow.
	if len(m.create.rows) != 3 {
		t.Fatalf("got %d rows, want 3: %+v", len(m.create.rows), m.create.rows)
	}
	remote := m.create.rows[2]
	if !remote.branch.IsRemote() || !remote.selectable() {
		t.Errorf("remote row = %+v, want remote and selectable", remote)
	}

	// Filtering by the remote prefix still finds it (full-name matching).
	m.create.input.SetValue("origin/rem")
	m.recomputeCreate()
	found := false
	for _, r := range m.create.rows[1:] {
		if r.branch.FullName() == "origin/remote-only" {
			found = true
		}
	}
	if !found {
		t.Errorf("filter %q did not match the remote branch: %+v", "origin/rem", m.create.rows)
	}
}
