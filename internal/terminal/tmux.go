package terminal

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func init() { Register(tmux{}) }

// tmux opens a new window in the current tmux session, or in the server if
// run outside a session.
type tmux struct{}

func (tmux) Name() string { return "tmux" }

func (tmux) Detect() bool { return os.Getenv("TMUX") != "" }

func (tmux) OpenTab(dir string, command ...string) error {
	args := []string{"new-window", "-c", dir}
	// tmux runs the trailing args as the window's command (argv-style, no shell
	// quoting needed); with no command it starts the default shell.
	args = append(args, command...)
	cmd := exec.Command("tmux", args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("tmux: %s", strings.TrimSpace(string(out)))
	}
	return nil
}
