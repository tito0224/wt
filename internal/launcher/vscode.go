package launcher

func init() { Register(vscode{}) }

// vscode opens a directory in Visual Studio Code.
type vscode struct{}

func (vscode) Name() string  { return "code" }
func (vscode) Title() string { return "VS Code" }
func (vscode) Icon() string  { return "\ue70c" } // nf-dev-visualstudio
func (vscode) Kind() Kind    { return KindGUI }

func (vscode) Available() bool {
	return hasBinary("code") || macAppInstalled("com.microsoft.VSCode")
}

func (vscode) Command(string) []string { return nil }
func (vscode) URL(string) string       { return "" }

func (vscode) GUILaunch(dir string) error {
	if hasBinary("code") {
		return runDetached("code", dir)
	}
	// Fall back to launching the app bundle with the folder as an argument.
	return runDetached("open", "-b", "com.microsoft.VSCode", dir)
}
