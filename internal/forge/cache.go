package forge

import "sync"

// cacheKey identifies a cached lookup.
type cacheKey struct {
	dir    string
	branch string
}

// cacheEntry records a completed lookup. found is false when the adapter
// returned no PR for the branch (a valid, cacheable outcome distinct from "not
// yet looked up").
type cacheEntry struct {
	pr    *PR
	found bool
}

var (
	cacheMu sync.Mutex
	cache   = map[cacheKey]cacheEntry{}

	// detectMu guards a small per-dir adapter cache so we resolve the forge
	// (and its CLI/remote checks) once per repository rather than per lookup.
	detectMu    sync.Mutex
	detectCache = map[string]Adapter{}
	detectDone  = map[string]bool{}
)

// adapterFor returns the detected adapter for dir, memoizing the result
// (including a nil "no forge" outcome).
func adapterFor(dir string) Adapter {
	detectMu.Lock()
	defer detectMu.Unlock()
	if detectDone[dir] {
		return detectCache[dir]
	}
	a := Detect(dir)
	detectCache[dir] = a
	detectDone[dir] = true
	return a
}

// LookupPR resolves the PR for branch in the repository at dir using the
// detected forge adapter. The returned PR is nil when no forge is detected, the
// branch has no associated PR, or the lookup fails. found reports whether a
// definitive answer was obtained.
//
// Results (including a definitive "no PR") are cached in memory for the process
// lifetime, since forge CLI calls are comparatively slow (network round-trip).
func LookupPR(dir, branch string) (*PR, bool) {
	if branch == "" {
		return nil, false
	}
	a := adapterFor(dir)
	if a == nil {
		return nil, false
	}

	key := cacheKey{dir: dir, branch: branch}
	cacheMu.Lock()
	if e, ok := cache[key]; ok {
		cacheMu.Unlock()
		return e.pr, e.found
	}
	cacheMu.Unlock()

	pr, found := a.PR(dir, branch)

	cacheMu.Lock()
	cache[key] = cacheEntry{pr: pr, found: found}
	cacheMu.Unlock()
	return pr, found
}
