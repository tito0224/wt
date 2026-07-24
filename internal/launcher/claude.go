package launcher

func init() { Register(claude{}) }

// claude runs Claude Code in a terminal in the worktree directory.
type claude struct{}

func (claude) Name() string    { return "claude" }
func (claude) Title() string   { return "Claude Code" }
func (claude) Icon() string    { return "\U000f0e79" } // nf-md-robot
func (claude) Kind() Kind      { return KindTerminal }
func (claude) Available() bool { return hasBinary("claude") }
func (claude) Command(string) []string {
	return []string{"claude"}
}
func (claude) GUILaunch(string) error { return nil }
func (claude) URL(string) string      { return "" }
