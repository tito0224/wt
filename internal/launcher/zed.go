package launcher

func init() { Register(zed{}) }

// zed opens a directory in the Zed editor.
type zed struct{}

func (zed) Name() string  { return "zed" }
func (zed) Title() string { return "Zed" }
func (zed) Icon() string  { return "\uf121" } // nf-fa-code (generic editor)
func (zed) Kind() Kind    { return KindGUI }

func (zed) Available() bool {
	return hasBinary("zed") || macAppInstalled("dev.zed.Zed")
}

func (zed) Command(string) []string { return nil }
func (zed) URL(string) string       { return "" }

func (zed) GUILaunch(dir string) error {
	if hasBinary("zed") {
		return runDetached("zed", dir)
	}
	return runDetached("open", "-b", "dev.zed.Zed", dir)
}
