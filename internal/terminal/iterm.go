package terminal

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func init() {
	Register(iTerm2{})
	Register(appleTerminal{})
	Register(tmux{})
	Register(ghostty{})
	Register(wezterm{})
}

// shellQuote wraps s in single quotes safe for POSIX shells.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// osaQuote escapes a string for embedding in an AppleScript double-quoted
// literal.
func osaQuote(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return `"` + s + `"`
}

func runOsascript(script string) error {
	cmd := exec.Command("osascript", "-e", script)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("osascript: %s", strings.TrimSpace(string(out)))
	}
	return nil
}

// iTerm2 opens a new tab in the current iTerm2 window via AppleScript.
type iTerm2 struct{}

func (iTerm2) Name() string { return "iterm" }

func (iTerm2) Detect() bool { return os.Getenv("TERM_PROGRAM") == "iTerm.app" }

func (iTerm2) OpenTab(dir string) error {
	cd := "cd " + shellQuote(dir)
	script := fmt.Sprintf(`
tell application "iTerm2"
	activate
	tell current window
		create tab with default profile
		tell current session to write text %s
	end tell
end tell`, osaQuote(cd))
	return runOsascript(script)
}

// appleTerminal opens a new tab in macOS Terminal.app.
type appleTerminal struct{}

func (appleTerminal) Name() string { return "terminal" }

func (appleTerminal) Detect() bool { return os.Getenv("TERM_PROGRAM") == "Apple_Terminal" }

func (appleTerminal) OpenTab(dir string) error {
	cd := "cd " + shellQuote(dir)
	// Open a new tab in the frontmost window using the Cmd-T keystroke, then
	// run the cd. This is the most reliable approach for Terminal.app.
	script := fmt.Sprintf(`
tell application "Terminal"
	activate
	tell application "System Events" to keystroke "t" using command down
	delay 0.2
	do script %s in front window
end tell`, osaQuote(cd))
	return runOsascript(script)
}

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

// ghostty opens a new window in the given directory via its CLI/open.
type ghostty struct{}

func (ghostty) Name() string { return "ghostty" }

func (ghostty) Detect() bool { return os.Getenv("TERM_PROGRAM") == "ghostty" }

func (ghostty) OpenTab(dir string) error {
	// Ghostty supports opening a new window at a working directory via `open`.
	cmd := exec.Command("open", "-na", "Ghostty", "--args", "--working-directory="+dir)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("ghostty: %s", strings.TrimSpace(string(out)))
	}
	return nil
}

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
