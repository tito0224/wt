package forge

import (
	"errors"
	"sync"
)

// cacheKey identifies a cached lookup.
type cacheKey struct {
	dir    string
	branch string
}

// cacheEntry records a completed lookup: the PR (nil for a definitive "no PR")
// and the error (ErrNoPR when there is none). Only definitive results are
// cached — transient failures are not, so they're retried.
type cacheEntry struct {
	pr  *PR
	err error
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
// detected forge adapter. It returns:
//
//   - (pr, nil)        the branch has a PR
//   - (nil, ErrNoPR)   no forge detected, or the branch has no PR (definitive)
//   - (nil, otherErr)  the lookup failed (transient; not cached)
//
// Definitive results (a PR, or ErrNoPR) are cached in memory for the process
// lifetime, since forge CLI calls are comparatively slow (network round-trip).
func LookupPR(dir, branch string) (*PR, error) {
	if branch == "" {
		return nil, ErrNoPR
	}
	a := adapterFor(dir)
	if a == nil {
		return nil, ErrNoPR
	}

	key := cacheKey{dir: dir, branch: branch}
	cacheMu.Lock()
	if e, ok := cache[key]; ok {
		cacheMu.Unlock()
		return e.pr, e.err
	}
	cacheMu.Unlock()

	pr, err := a.PR(dir, branch)

	// Cache definitive outcomes only (a PR, or a definitive ErrNoPR). Transient
	// failures are left uncached so a later selection retries.
	if err == nil || errors.Is(err, ErrNoPR) {
		cacheMu.Lock()
		cache[key] = cacheEntry{pr: pr, err: err}
		cacheMu.Unlock()
	}
	return pr, err
}
