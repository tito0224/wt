package tui

import (
	"fmt"
	"os/exec"
	"runtime"
)

// openURL opens rawURL in the user's default browser using the platform opener
// (macOS "open", otherwise "xdg-open"). It returns an error if the opener is
// missing or fails.
func openURL(rawURL string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", rawURL)
	default:
		cmd = exec.Command("xdg-open", rawURL)
	}
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("open url: %s", string(out))
	}
	return nil
}
