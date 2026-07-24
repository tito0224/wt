package launcher

import (
	"encoding/base64"
	"os"
	"strings"
	"sync"
)

func init() {
	Register(opencode{})
	Register(opencodeWeb{})
}

// opencode runs the Opencode CLI in a terminal in the worktree directory.
type opencode struct{}

func (opencode) Name() string            { return "opencode" }
func (opencode) Title() string           { return "Opencode (CLI)" }
func (opencode) Icon() string            { return "\U000f0e79" } // nf-md-robot
func (opencode) Kind() Kind              { return KindTerminal }
func (opencode) Available() bool         { return hasBinary("opencode") }
func (opencode) Command(string) []string { return []string{"opencode"} }
func (opencode) GUILaunch(string) error  { return nil }
func (opencode) URL(string) string       { return "" }

// opencodeWebOrigin is the origin of an already-running `opencode web` server.
// When set (via config or the OPENCODE_WEB_ORIGIN env var), the opencode-web
// launcher opens a deep link into that server rather than starting a new one.
var (
	owMu     sync.RWMutex
	owOrigin string
)

// SetOpencodeWebOrigin configures the running opencode-web server origin (e.g.
// "http://localhost:4096"). An empty value clears it. Callers typically set
// this once at startup from config.
func SetOpencodeWebOrigin(origin string) {
	owMu.Lock()
	owOrigin = strings.TrimRight(strings.TrimSpace(origin), "/")
	owMu.Unlock()
}

// opencodeWebOriginValue returns the effective origin: the configured value, or
// the OPENCODE_WEB_ORIGIN env var (which the opencode web plugin also uses).
func opencodeWebOriginValue() string {
	owMu.RLock()
	o := owOrigin
	owMu.RUnlock()
	if o == "" {
		o = strings.TrimRight(strings.TrimSpace(os.Getenv("OPENCODE_WEB_ORIGIN")), "/")
	}
	return o
}

// opencodeWebDeepLink builds the opencode web deep link that opens a session
// bound to dir: <origin>/<base64url(dir)>/session. This mirrors the opencode
// web client's route scheme (see the worktree opencode plugin).
func opencodeWebDeepLink(origin, dir string) string {
	b64 := base64.RawURLEncoding.EncodeToString([]byte(dir))
	return origin + "/" + b64 + "/session"
}

// opencodeWeb opens the Opencode web UI for a worktree. When an origin is
// configured (config/env), it is a URL launcher that deep-links into the
// already-running server; otherwise it starts a fresh `opencode web` in a new
// terminal.
type opencodeWeb struct{}

func (opencodeWeb) Name() string    { return "opencode-web" }
func (opencodeWeb) Title() string   { return "Opencode (web)" }
func (opencodeWeb) Icon() string    { return "\U000f059f" } // nf-md-web
func (opencodeWeb) Available() bool { return opencodeWebOriginValue() != "" || hasBinary("opencode") }

// Kind is URL when a server origin is configured (open a deep link), else
// Terminal (start a new `opencode web`).
func (opencodeWeb) Kind() Kind {
	if opencodeWebOriginValue() != "" {
		return KindURL
	}
	return KindTerminal
}

func (opencodeWeb) Command(string) []string { return []string{"opencode", "web"} }
func (opencodeWeb) GUILaunch(string) error  { return nil }

func (opencodeWeb) URL(dir string) string {
	origin := opencodeWebOriginValue()
	if origin == "" {
		return ""
	}
	return opencodeWebDeepLink(origin, dir)
}
