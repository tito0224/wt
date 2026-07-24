package tui

import (
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/johnlotito/wt/internal/git"
)

func TestOrdering(t *testing.T) {
	initStyles(lipgloss.DefaultRenderer())
	all := []git.Worktree{
		{Path: "/r/root", Branch: "root", Current: true},
		{Path: "/r/feature-payments", Branch: "feature-payments"},
		{Path: "/r/pay", Branch: "pay"},
		{Path: "/r/hotfix", Branch: "hotfix"},
	}
	m := &model{width: 100, height: 24, all: all}

	// Unfiltered: git-native order preserved.
	m.recompute()
	want := []string{"root", "feature-payments", "pay", "hotfix"}
	for i, w := range want {
		if m.rows[i].wt.Branch != w {
			t.Fatalf("unfiltered row %d = %q, want %q", i, m.rows[i].wt.Branch, w)
		}
	}

	// Filtered by "pay": both "pay" and "feature-payments" match; exact/shorter
	// "pay" should rank first by score.
	m.filter = "pay"
	m.recompute()
	if len(m.rows) != 2 {
		t.Fatalf("expected 2 matches, got %d", len(m.rows))
	}
	if m.rows[0].wt.Branch != "pay" {
		t.Fatalf("filtered top = %q, want pay (higher score)", m.rows[0].wt.Branch)
	}
	if m.rows[0].score <= m.rows[1].score {
		t.Fatalf("expected descending score, got %d then %d", m.rows[0].score, m.rows[1].score)
	}
}
