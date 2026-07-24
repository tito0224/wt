package launcher

func init() { Register(cursor{}) }

// cursor opens a directory in Cursor.
type cursor struct{}

func (cursor) Name() string  { return "cursor" }
func (cursor) Title() string { return "Cursor" }
func (cursor) Icon() string  { return "\uf121" } // nf-fa-code (generic editor)
func (cursor) Kind() Kind    { return KindGUI }

func (cursor) Available() bool {
	return hasBinary("cursor") || macAppInstalled("com.todesktop.230313mzl4w4u92") // Cursor bundle id
}

func (cursor) Command(string) []string { return nil }
func (cursor) URL(string) string       { return "" }

func (cursor) GUILaunch(dir string) error {
	if hasBinary("cursor") {
		return runDetached("cursor", dir)
	}
	return runDetached("open", "-b", "com.todesktop.230313mzl4w4u92", dir)
}
