package forge

import (
	"errors"
	"sync"
	"testing"
)

// fakeAdapter is a scriptable Adapter for cache tests. It detects every
// directory and answers PR lookups from the respond map, counting calls so
// tests can assert whether the cache short-circuited a lookup.
//
// The name sorts before "github" so Detect resolves this adapter first for the
// synthetic test dirs (which have no GitHub remote anyway).
type fakeAdapter struct {
	mu      sync.Mutex
	calls   map[string]int                 // dir+"\x00"+branch -> call count
	respond map[string]func() (*PR, error) // dir -> canned response
}

func (f *fakeAdapter) Name() string           { return "fake-test-forge" }
func (f *fakeAdapter) Detect(dir string) bool { return true }

func (f *fakeAdapter) PR(dir, branch string) (*PR, error) {
	f.mu.Lock()
	f.calls[dir+"\x00"+branch]++
	fn := f.respond[dir]
	f.mu.Unlock()
	if fn == nil {
		return nil, ErrNoPR
	}
	return fn()
}

func (f *fakeAdapter) callCount(dir, branch string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls[dir+"\x00"+branch]
}

// The registry is global and panics on duplicates, so the fake is registered
// exactly once for the whole test binary.
var (
	fakeOnce sync.Once
	fake     = &fakeAdapter{
		calls:   map[string]int{},
		respond: map[string]func() (*PR, error){},
	}
)

func registerFake() {
	fakeOnce.Do(func() { Register(fake) })
}

func TestLookupPRCachesFoundPR(t *testing.T) {
	registerFake()
	// A distinct dir per test keeps the global lookup/detect caches from
	// leaking state between subtests.
	dir := t.TempDir()
	want := &PR{Number: 7, Title: "add tests", State: "OPEN"}
	fake.respond[dir] = func() (*PR, error) { return want, nil }

	for i := 0; i < 2; i++ {
		pr, err := LookupPR(dir, "feature")
		if err != nil {
			t.Fatalf("LookupPR call %d: %v", i+1, err)
		}
		if pr != want {
			t.Fatalf("LookupPR call %d = %+v; want %+v", i+1, pr, want)
		}
	}
	if n := fake.callCount(dir, "feature"); n != 1 {
		t.Errorf("adapter called %d times; want 1 (second lookup should hit the cache)", n)
	}
}

func TestLookupPRCachesErrNoPR(t *testing.T) {
	registerFake()
	dir := t.TempDir()
	fake.respond[dir] = func() (*PR, error) { return nil, ErrNoPR }

	for i := 0; i < 2; i++ {
		pr, err := LookupPR(dir, "feature")
		if !errors.Is(err, ErrNoPR) {
			t.Fatalf("LookupPR call %d = (%v, %v); want ErrNoPR", i+1, pr, err)
		}
	}
	if n := fake.callCount(dir, "feature"); n != 1 {
		t.Errorf("adapter called %d times; want 1 (ErrNoPR is definitive and cached)", n)
	}
}

func TestLookupPRDoesNotCacheTransientError(t *testing.T) {
	registerFake()
	dir := t.TempDir()
	transient := errors.New("gh pr view feature: network unreachable")
	fake.respond[dir] = func() (*PR, error) { return nil, transient }

	for i := 0; i < 2; i++ {
		if _, err := LookupPR(dir, "feature"); !errors.Is(err, transient) {
			t.Fatalf("LookupPR call %d err = %v; want %v", i+1, err, transient)
		}
	}
	if n := fake.callCount(dir, "feature"); n != 2 {
		t.Errorf("adapter called %d times; want 2 (transient errors must be retried)", n)
	}
}

func TestLookupPREmptyBranch(t *testing.T) {
	registerFake()
	dir := t.TempDir()

	pr, err := LookupPR(dir, "")
	if pr != nil || !errors.Is(err, ErrNoPR) {
		t.Errorf("LookupPR(dir, \"\") = (%v, %v); want (nil, ErrNoPR)", pr, err)
	}
	if n := fake.callCount(dir, ""); n != 0 {
		t.Errorf("adapter called %d times for empty branch; want 0", n)
	}
}
