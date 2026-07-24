package main

import (
	"strings"
	"testing"

	"github.com/tito0224/wt/internal/git"
)

func TestFindWorktree(t *testing.T) {
	wts := []git.Worktree{
		{Branch: "main", Path: "/repos/wt"},
		{Branch: "feature", Path: "/repos/wt-worktrees/feature"},
		{Branch: "feature-extra", Path: "/repos/wt-worktrees/feature-extra"},
		{Branch: "bugfix", Path: "/repos/wt-worktrees/bugfix"},
	}

	t.Run("exact branch match wins over substrings", func(t *testing.T) {
		// "feature" is a substring of both feature branches (and their paths);
		// the exact branch match must win instead of reporting ambiguity.
		got, err := findWorktree(wts, "feature")
		if err != nil {
			t.Fatalf("findWorktree: %v", err)
		}
		if got.Branch != "feature" {
			t.Errorf("matched branch %q; want feature", got.Branch)
		}
	})

	t.Run("single substring match", func(t *testing.T) {
		got, err := findWorktree(wts, "bug")
		if err != nil {
			t.Fatalf("findWorktree: %v", err)
		}
		if got.Branch != "bugfix" {
			t.Errorf("matched branch %q; want bugfix", got.Branch)
		}
	})

	t.Run("ambiguous lists candidates", func(t *testing.T) {
		_, err := findWorktree(wts, "feat")
		if err == nil {
			t.Fatal("findWorktree(feat) succeeded; want ambiguity error")
		}
		msg := err.Error()
		if !strings.Contains(msg, "ambiguous") {
			t.Errorf("error %q does not say ambiguous", msg)
		}
		for _, want := range []string{"feature", "feature-extra"} {
			if !strings.Contains(msg, want) {
				t.Errorf("error %q does not list candidate %q", msg, want)
			}
		}
	})

	t.Run("no match", func(t *testing.T) {
		_, err := findWorktree(wts, "nonexistent")
		if err == nil {
			t.Fatal("findWorktree(nonexistent) succeeded; want error")
		}
		if !strings.Contains(err.Error(), "nonexistent") {
			t.Errorf("error %q does not mention the query", err)
		}
	})
}
