package terminal

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func init() { Register(wezterm{}) }

// wezterm opens a new tab using the wezterm CLI.
type wezterm struct{}

func (wezterm) Name() string { return "wezterm" }

func (wezterm) Detect() bool { return os.Getenv("TERM_PROGRAM") == "WezTerm" }

func (wezterm) OpenTab(dir string) error {
	cmd := exec.Command("wezterm", "cli", "spawn", "--cwd", dir)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("wezterm: %s", strings.TrimSpace(string(out)))
	}
	return nil
}
