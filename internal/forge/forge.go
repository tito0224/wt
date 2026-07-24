// Package forge defines an adapter interface for looking up the pull/merge
// request associated with a branch on a git hosting provider ("forge"), plus
// built-in adapters. Add support for a new forge (e.g. GitLab) by implementing
// Adapter and registering it.
//
// Lookups are best-effort: when no adapter detects, its CLI is missing, or the
// branch has no request, callers get a nil PR rather than an error.
package forge

import "sort"

// PR is the subset of pull/merge-request fields wt displays.
type PR struct {
	Number  int
	Title   string
	State   string // OPEN, MERGED, CLOSED (uppercased by adapters)
	URL     string
	IsDraft bool
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
	// PR returns the request for branch in the repository at dir. The returned
	// PR is nil when the branch has no associated request. found reports whether
	// a definitive answer was obtained, so callers can distinguish "no PR" from
	// "lookup failed" (and avoid caching failures as "none").
	PR(dir, branch string) (pr *PR, found bool)
}

var registry = map[string]Adapter{}

// Register adds an adapter to the global registry. Intended for use from
// init(). Panics on duplicate names to catch programming errors early.
func Register(a Adapter) {
	if _, dup := registry[a.Name()]; dup {
		panic("forge: duplicate adapter " + a.Name())
	}
	registry[a.Name()] = a
}

// Names returns the sorted list of registered adapter names.
func Names() []string {
	names := make([]string, 0, len(registry))
	for n := range registry {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// Detect returns the first adapter that detects the repository at dir, or nil
// when none applies (e.g. not a GitHub repo, or gh not installed).
func Detect(dir string) Adapter {
	for _, name := range Names() { // deterministic order
		a := registry[name]
		if a.Detect(dir) {
			return a
		}
	}
	return nil
}
