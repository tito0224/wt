package git

import (
	"fmt"
	"strings"
)

// Label returns the display name for a worktree: its branch, or a short SHA
// when detached, or "(bare)" for the bare entry.
func (w Worktree) Label() string {
	switch {
	case w.Bare:
		return "(bare)"
	case w.Detached:
		if len(w.Head) >= 7 {
			return "detached@" + w.Head[:7]
		}
		return "detached"
	default:
		return w.Branch
	}
}

// DisplayLabel is Label with a trailing " *" marking the current worktree,
// mirroring the `git branch` convention.
func (w Worktree) DisplayLabel() string {
	if w.Current {
		return w.Label() + " *"
	}
	return w.Label()
}

// StatusTags returns short human-readable status markers, e.g. ["dirty",
// "↑2", "↓1", "↥3"]. The ↑/↓ markers are ahead/behind the branch's own
// upstream; ↥ is commits ahead of the default branch (see AheadOfBase — a
// neutral count, not a claim that the work is unmerged).
func (w Worktree) StatusTags() []string {
	if w.Bare {
		return nil
	}
	var tags []string
	if w.Dirty {
		tags = append(tags, "dirty")
	} else {
		tags = append(tags, "clean")
	}
	if w.Ahead > 0 {
		tags = append(tags, fmt.Sprintf("↑%d", w.Ahead))
	}
	if w.Behind > 0 {
		tags = append(tags, fmt.Sprintf("↓%d", w.Behind))
	}
	if w.AheadOfBase > 0 {
		tags = append(tags, fmt.Sprintf("↥%d", w.AheadOfBase))
	}
	if w.Locked {
		tags = append(tags, "locked")
	}
	if w.Prunable {
		tags = append(tags, "prunable")
	}
	return tags
}

// StatusString joins StatusTags with commas.
func (w Worktree) StatusString() string {
	return strings.Join(w.StatusTags(), ", ")
}
