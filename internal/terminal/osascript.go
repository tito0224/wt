package terminal

import (
	"fmt"
	"os/exec"
	"strings"
)

// shellQuote wraps s in single quotes safe for POSIX shells.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// cdCommandLine builds a POSIX shell line that cds into dir and, when command
// is non-empty, execs it there. Each argument is individually shell-quoted.
func cdCommandLine(dir string, command []string) string {
	line := "cd " + shellQuote(dir)
	if len(command) > 0 {
		parts := make([]string, len(command))
		for i, a := range command {
			parts[i] = shellQuote(a)
		}
		line += " && " + strings.Join(parts, " ")
	}
	return line
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
