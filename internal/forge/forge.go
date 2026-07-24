// Package forge defines an adapter interface for looking up the pull/merge
// request associated with a branch on a git hosting provider ("forge"), plus
// built-in adapters. Add support for a new forge (e.g. GitLab) by implementing
// Adapter and registering it.
//
// Lookups are best-effort: when no adapter detects, its CLI is missing, or the
// branch has no request, callers get a nil PR rather than an error.
package forge

import (
	"errors"

	"github.com/johnlotito/wt/internal/registry"
)

// ErrNoPR is returned by Adapter.PR / LookupPR when the lookup succeeded but
// the branch has no associated pull/merge request. This is a definitive answer
// (distinct from a network or CLI failure) and is safe to cache. Use
// errors.Is(err, ErrNoPR) to test for it.
var ErrNoPR = errors.New("forge: no pull request for branch")

// PR is the subset of pull/merge-request fields wt displays.
type PR struct {
	Number  int
	Title   string
	State   string // OPEN, MERGED, CLOSED (uppercased by adapters)
	URL     string
	IsDraft bool
	// MergedAt is the merge time (RFC3339) when State is MERGED, else empty.
	// Used to detect commits pushed to the branch after it was merged.
	MergedAt string
}

// Adapter knows how to resolve the pull/merge request for a branch in a
// repository, using a hosting provider's CLI.
//
// Implement this interface and call Register in an init() function to add
// support for a new forge.
type Adapter interface {
	// Name is the stable identifier for the adapter (e.g. "github").
	Name() string
	// Detect reports whether this adapter is appropriate for the repository at
	// dir, typically by inspecting the git remote host and checking that the
	// provider's CLI is installed.
	Detect(dir string) bool
	// PR returns the request for branch in the repository at dir. It returns
	// ErrNoPR when the lookup succeeded but there is no PR (a definitive,
	// cacheable answer), or another error when the lookup itself failed.
	PR(dir, branch string) (*PR, error)
}

var adapters = registry.New[Adapter]("forge", "adapter")

// Register adds an adapter to the global registry. Intended for use from
// init(). Panics on duplicate names to catch programming errors early.
func Register(a Adapter) { adapters.Register(a.Name(), a) }

// Names returns the sorted list of registered adapter names.
func Names() []string { return adapters.Names() }

// Detect returns the first adapter that detects the repository at dir, or nil
// when none applies (e.g. not a GitHub repo, or gh not installed). Adapters
// are tried in sorted-name order so the outcome is deterministic.
func Detect(dir string) Adapter {
	for _, a := range adapters.Values() {
		if a.Detect(dir) {
			return a
		}
	}
	return nil
}
