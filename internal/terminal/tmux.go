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

func (tmux) OpenTab(dir string) error {
	cmd := exec.Command("tmux", "new-window", "-c", dir)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("tmux: %s", strings.TrimSpace(string(out)))
	}
	return nil
}
