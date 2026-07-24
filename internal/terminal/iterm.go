package terminal

import (
	"fmt"
	"os"
)

func init() { Register(iTerm2{}) }

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
