package launcher

// This file registers terminal-based AI coding agents that run as a CLI in the
// worktree directory. They all share the same shape (run `<bin>` in a new
// terminal), so they're expressed with a small agentCLI helper rather than a
// bespoke type each.

func init() {
	Register(agentCLI{name: "copilot", title: "GitHub Copilot", bin: "copilot"})
	Register(agentCLI{name: "codex", title: "Codex", bin: "codex"})
	Register(agentCLI{name: "gemini", title: "Gemini", bin: "gemini"})
}

// agentCLI is a terminal AI agent launched by running its binary in the
// worktree directory. args are extra arguments appended after the binary.
type agentCLI struct {
	name  string
	title string
	bin   string
	args  []string
}

func (a agentCLI) Name() string  { return a.name }
func (a agentCLI) Title() string { return a.title }
func (a agentCLI) Icon() string  { return "\U000f0e79" } // nf-md-robot
func (a agentCLI) Kind() Kind    { return KindTerminal }

func (a agentCLI) Available() bool { return hasBinary(a.bin) }

func (a agentCLI) Command(string) []string {
	return append([]string{a.bin}, a.args...)
}

func (agentCLI) GUILaunch(string) error { return nil }
func (agentCLI) URL(string) string      { return "" }
