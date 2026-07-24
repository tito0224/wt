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
	Number   int    `json:"number"`
	Title    string `json:"title"`
	State    string `json:"state"`
	URL      string `json:"url"`
	IsDraft  bool   `json:"isDraft"`
	MergedAt string `json:"mergedAt"`
}

func (github) PR(dir, branch string) (*PR, error) {
	cmd := exec.Command("gh", "pr", "view", branch,
		"--json", "number,title,state,url,isDraft,mergedAt")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		// gh exits non-zero when there is no PR for the branch. It also does so
		// on real failures (auth, network), but from a single `pr view` we
		// can't reliably tell them apart, so treat non-zero as "no PR" — a
		// definitive, cacheable answer. (Detect already gated on a GitHub
		// remote + gh being installed.)
		return nil, ErrNoPR
	}
	var p ghPR
	if err := json.Unmarshal(out, &p); err != nil {
		return nil, err
	}
	return &PR{
		Number:   p.Number,
		Title:    p.Title,
		State:    strings.ToUpper(p.State),
		URL:      p.URL,
		IsDraft:  p.IsDraft,
		MergedAt: p.MergedAt,
	}, nil
}
