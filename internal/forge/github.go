package forge

import (
	"encoding/json"
	"os/exec"
	"strings"
	"sync"

	"github.com/johnlotito/wt/internal/git"
)

func init() { Register(github{}) }

// github resolves pull requests via the GitHub CLI (gh).
type github struct{}

func (github) Name() string { return "github" }

// ghAvailable reports whether the gh binary is installed. Computed once.
var (
	ghOnce sync.Once
	ghHave bool
)

func ghAvailable() bool {
	ghOnce.Do(func() {
		_, err := exec.LookPath("gh")
		ghHave = err == nil
	})
	return ghHave
}

// Detect reports whether dir has a github.com remote and the gh CLI is present.
func (github) Detect(dir string) bool {
	if !ghAvailable() {
		return false
	}
	url := git.RemoteURL(dir, "origin")
	return strings.Contains(url, "github.com")
}

// ghPR mirrors the JSON returned by `gh pr view --json ...`.
type ghPR struct {
	Number  int    `json:"number"`
	Title   string `json:"title"`
	State   string `json:"state"`
	URL     string `json:"url"`
	IsDraft bool   `json:"isDraft"`
}

func (github) PR(dir, branch string) (*PR, bool) {
	cmd := exec.Command("gh", "pr", "view", branch,
		"--json", "number,title,state,url,isDraft")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		// gh exits non-zero when there is no PR (or not a GitHub repo); treat
		// that as a definitive "no PR" so we don't retry every selection.
		return nil, true
	}
	var p ghPR
	if err := json.Unmarshal(out, &p); err != nil {
		return nil, false
	}
	return &PR{
		Number:  p.Number,
		Title:   p.Title,
		State:   strings.ToUpper(p.State),
		URL:     p.URL,
		IsDraft: p.IsDraft,
	}, true
}
