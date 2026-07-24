package terminal

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func init() { Register(ghostty{}) }

// ghostty opens a new window in the given directory via its CLI/open.
type ghostty struct{}

func (ghostty) Name() string { return "ghostty" }

func (ghostty) Detect() bool { return os.Getenv("TERM_PROGRAM") == "ghostty" }

func (ghostty) OpenTab(dir string, command ...string) error {
	// Ghostty opens a new window at a working directory via `open`. With a
	// command, `-e <argv...>` runs it in that window.
	args := []string{"-na", "Ghostty", "--args", "--working-directory=" + dir}
	if len(command) > 0 {
		args = append(args, "-e")
		args = append(args, command...)
	}
	cmd := exec.Command("open", args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("ghostty: %s", strings.TrimSpace(string(out)))
	}
	return nil
}
