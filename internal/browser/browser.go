// Package browser opens URLs in the user's default browser via the platform
// opener (macOS `open`, otherwise `xdg-open`). It exists so the TUI and the
// launcher package share one implementation.
package browser

import (
	"fmt"
	"os/exec"
	"runtime"
)

// Open launches the platform's URL opener for rawURL without waiting for it
// (fire-and-forget). It returns an error only when the opener fails to start;
// failures inside the browser itself are not observable.
func Open(rawURL string) error {
	opener := "xdg-open"
	if runtime.GOOS == "darwin" {
		opener = "open"
	}
	cmd := exec.Command(opener, rawURL)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("%s %s: %w", opener, rawURL, err)
	}
	// Release the process so we don't accumulate zombies; the exit status is
	// intentionally ignored.
	go func() { _ = cmd.Wait() }()
	return nil
}
