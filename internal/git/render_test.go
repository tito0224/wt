package git

import (
	"reflect"
	"testing"
)

func TestWorktreeLabel(t *testing.T) {
	tests := []struct {
		name string
		w    Worktree
		want string
	}{
		{"bare", Worktree{Bare: true}, "(bare)"},
		{"branch", Worktree{Branch: "feat/x"}, "feat/x"},
		{"detached long sha", Worktree{Detached: true, Head: "abcdef0123456789"}, "detached@abcdef0"},
		// A HEAD shorter than 7 chars can't be abbreviated; Label degrades to
		// the plain "detached" marker instead of slicing out of range.
		{"detached short sha", Worktree{Detached: true, Head: "abc"}, "detached"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.w.Label(); got != tt.want {
				t.Errorf("Label() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestWorktreeDisplayLabel(t *testing.T) {
	w := Worktree{Branch: "main"}
	if got := w.DisplayLabel(); got != "main" {
		t.Errorf("DisplayLabel() = %q, want %q", got, "main")
	}
	w.Current = true
	if got := w.DisplayLabel(); got != "main *" {
		t.Errorf("DisplayLabel() with Current = %q, want %q", got, "main *")
	}
}

func TestWorktreeStatusTags(t *testing.T) {
	tests := []struct {
		name string
		w    Worktree
		want []string
	}{
		// The bare entry has no working files, so no status applies at all.
		{"bare", Worktree{Bare: true, Dirty: true}, nil},
		{"clean", Worktree{}, []string{"clean"}},
		{"dirty", Worktree{Dirty: true}, []string{"dirty"}},
		{
			"everything",
			Worktree{Dirty: true, Ahead: 2, Behind: 1, AheadOfBase: 3, Locked: true, Prunable: true},
			[]string{"dirty", "↑2", "↓1", "↥3", "locked", "prunable"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.w.StatusTags(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("StatusTags() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestWorktreeStatusString(t *testing.T) {
	w := Worktree{Dirty: true, Ahead: 1}
	if got := w.StatusString(); got != "dirty, ↑1" {
		t.Errorf("StatusString() = %q, want %q", got, "dirty, ↑1")
	}
	if got := (Worktree{Bare: true}).StatusString(); got != "" {
		t.Errorf("bare StatusString() = %q, want empty", got)
	}
}
