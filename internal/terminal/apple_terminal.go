package terminal

import (
	"fmt"
	"os"
)

func init() { Register(appleTerminal{}) }

// appleTerminal opens a new tab in macOS Terminal.app.
type appleTerminal struct{}

func (appleTerminal) Name() string { return "terminal" }

func (appleTerminal) Detect() bool { return os.Getenv("TERM_PROGRAM") == "Apple_Terminal" }

func (appleTerminal) OpenTab(dir string, command ...string) error {
	line := cdCommandLine(dir, command)
	// Open a new tab in the frontmost window using the Cmd-T keystroke, then
	// run the command. This is the most reliable approach for Terminal.app.
	script := fmt.Sprintf(`
tell application "Terminal"
	activate
	tell application "System Events" to keystroke "t" using command down
	delay 0.2
	do script %s in front window
end tell`, osaQuote(line))
	return runOsascript(script)
}
