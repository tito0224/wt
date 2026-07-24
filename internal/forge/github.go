package forge

import (
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"sync"

	"github.com/tito0224/wt/internal/git"
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
		// gh exits non-zero both when the branch has no PR and on real
		// failures (auth, network). Distinguish them by stderr: only the
		// definitive "no pull requests found" message maps to ErrNoPR (which
		// the session cache stores permanently). Anything else — including a
		// non-ExitError, e.g. gh disappearing mid-run — is reported as a real
		// error so the cache treats it as transient and retries later, rather
		// than a flaky network blip hiding PR info for the whole session.
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			stderr := strings.TrimSpace(string(exitErr.Stderr))
			if strings.Contains(strings.ToLower(stderr), "no pull requests found") {
				return nil, ErrNoPR
			}
			return nil, fmt.Errorf("gh pr view %s: %s", branch, stderr)
		}
		return nil, fmt.Errorf("gh pr view %s: %w", branch, err)
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
