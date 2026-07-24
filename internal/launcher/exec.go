package launcher

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"sync"
)

// hasBinary reports whether name is an executable on PATH. Results are memoized
// since PATH lookups don't change during a run.
var (
	binMu    sync.Mutex
	binCache = map[string]bool{}
)

func hasBinary(name string) bool {
	binMu.Lock()
	defer binMu.Unlock()
	if v, ok := binCache[name]; ok {
		return v
	}
	_, err := exec.LookPath(name)
	ok := err == nil
	binCache[name] = ok
	return ok
}

// macAppInstalled reports whether a macOS application bundle with the given
// bundle identifier is installed, by querying the Spotlight index via mdfind.
// Returns false on non-macOS.
func macAppInstalled(bundleID string) bool {
	if runtime.GOOS != "darwin" {
		return false
	}
	out, err := exec.Command("mdfind",
		fmt.Sprintf("kMDItemCFBundleIdentifier == %q", bundleID)).Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) != ""
}

// runDetached starts cmd without waiting for it, for fire-and-forget GUI
// launches. Errors from launching (not from the app itself) are returned.
func runDetached(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	// Release so we don't accumulate zombies; we don't care about the exit.
	go func() { _ = cmd.Wait() }()
	return nil
}
