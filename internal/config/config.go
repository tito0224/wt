// Package config loads user-configurable defaults for wt. Configuration is
// layered: built-in defaults are overlaid by a global config file
// ($XDG_CONFIG_HOME/wt/config.toml), then by a per-repo file (.wt.toml at the
// repository root), then by WT_* environment variables. Command-line flags,
// applied by callers, take final precedence.
package config

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/BurntSushi/toml"
	"github.com/johnlotito/wt/internal/git"
)

// exampleTemplate is the commented config file scaffold. It is a Go text
// template whose fields are populated from a Create value.
//
//go:embed config.toml.tmpl
var exampleTemplate string

// Create holds the defaults used when creating a new worktree.
type Create struct {
	// BaseBranch is the branch/ref new worktrees are created from. When empty,
	// the repository's default branch (origin/HEAD, then main/master) is used.
	BaseBranch string `toml:"base_branch"`
	// Dir is the directory new worktrees are placed in. It may be absolute or
	// relative to the repository root. When empty, "../<repo>-worktrees" is
	// used. It supports the {repo} placeholder.
	Dir string `toml:"dir"`
	// BranchPrefix is prepended to the worktree name to form the branch name,
	// joined with BranchSeparator (e.g. prefix "jl", separator "/" -> jl/foo).
	BranchPrefix string `toml:"branch_prefix"`
	// BranchSeparator joins the prefix and the name. Defaults to "/".
	BranchSeparator string `toml:"branch_separator"`
}

// Config is the top-level configuration.
type Config struct {
	Create Create `toml:"create"`
}

// Default returns the built-in defaults.
func Default() Config {
	return Config{
		Create: Create{
			BranchSeparator: "/",
		},
	}
}

// configHome returns $XDG_CONFIG_HOME, falling back to ~/.config.
func configHome() (string, error) {
	if v := os.Getenv("XDG_CONFIG_HOME"); v != "" {
		return v, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config"), nil
}

// GlobalPath returns the path to the global config file.
func GlobalPath() (string, error) {
	h, err := configHome()
	if err != nil {
		return "", err
	}
	return filepath.Join(h, "wt", "config.toml"), nil
}

// RepoPath returns the per-repo config path (.wt.toml at repoRoot). An empty
// repoRoot yields an empty path.
func RepoPath(repoRoot string) string {
	if repoRoot == "" {
		return ""
	}
	return filepath.Join(repoRoot, ".wt.toml")
}

// Load builds the effective configuration by layering, in increasing order of
// precedence: built-in defaults, the global file, the per-repo file, and
// WT_* environment variables. Missing files are not an error.
func Load(repoRoot string) (Config, error) {
	cfg := Default()

	if gp, err := GlobalPath(); err == nil {
		if err := mergeFile(&cfg, gp); err != nil {
			return cfg, err
		}
	}

	if rp := RepoPath(repoRoot); rp != "" {
		if err := mergeFile(&cfg, rp); err != nil {
			return cfg, err
		}
	}

	applyEnv(&cfg)
	return cfg, nil
}

// mergeFile decodes the TOML file at path over cfg. A missing file is ignored.
// Fields absent from the file retain their prior value because the decoder
// only overwrites keys present in the document.
func mergeFile(cfg *Config, path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read config %s: %w", path, err)
	}
	if err := toml.Unmarshal(data, cfg); err != nil {
		return fmt.Errorf("parse config %s: %w", path, err)
	}
	return nil
}

// applyEnv overlays WT_* environment variables when set.
func applyEnv(cfg *Config) {
	if v, ok := os.LookupEnv("WT_BASE_BRANCH"); ok {
		cfg.Create.BaseBranch = v
	}
	if v, ok := os.LookupEnv("WT_DIR"); ok {
		cfg.Create.Dir = v
	}
	if v, ok := os.LookupEnv("WT_BRANCH_PREFIX"); ok {
		cfg.Create.BranchPrefix = v
	}
	if v, ok := os.LookupEnv("WT_BRANCH_SEPARATOR"); ok {
		cfg.Create.BranchSeparator = v
	}
}

// CreateWorktree loads the effective config for the repository containing
// repoRoot and creates a new worktree named `name`. The base branch, target
// directory, and branch naming come from config; empty base falls back to the
// repository's default branch. It returns the resolved branch name and the
// absolute worktree path.
//
// The base and dir arguments, when non-empty, override the corresponding
// config values (mirroring the create command's flags).
func CreateWorktree(repoRoot, name, base, dir string) (branch, path string, err error) {
	cfg, err := Load(repoRoot)
	if err != nil {
		return "", "", err
	}
	if base != "" {
		cfg.Create.BaseBranch = base
	}
	if dir != "" {
		cfg.Create.Dir = dir
	}
	if cfg.Create.BaseBranch == "" {
		cfg.Create.BaseBranch = git.DefaultBranch(repoRoot)
	}

	branch = cfg.Create.BranchName(name)
	path = cfg.Create.WorktreeDir(repoRoot, name)
	if err := git.Add(repoRoot, path, branch, cfg.Create.BaseBranch); err != nil {
		return "", "", err
	}
	return branch, path, nil
}

// BranchName composes a branch name from the configured prefix, separator, and
// the given worktree name.
func (c Create) BranchName(name string) string {
	if c.BranchPrefix == "" {
		return name
	}
	sep := c.BranchSeparator
	if sep == "" {
		sep = "/"
	}
	return c.BranchPrefix + sep + name
}

// WorktreeDir resolves the directory a new worktree named `name` should live
// in, given the repository root. It expands the {repo} placeholder, resolves
// relative paths against repoRoot, and appends the worktree name as the final
// path element.
func (c Create) WorktreeDir(repoRoot, name string) string {
	repoName := filepath.Base(repoRoot)

	dir := c.Dir
	if dir == "" {
		dir = filepath.Join("..", repoName+"-worktrees")
	}
	dir = strings.ReplaceAll(dir, "{repo}", repoName)

	if !filepath.IsAbs(dir) {
		dir = filepath.Join(repoRoot, dir)
	}
	return filepath.Join(dir, name)
}

// Render returns a commented TOML document for the given config, suitable for
// writing to a config file. It expands the embedded example template with the
// config's values, preserving the template's documentation comments.
func Render(cfg Config) (string, error) {
	c := cfg.Create
	if c.BranchSeparator == "" {
		c.BranchSeparator = "/"
	}
	t, err := template.New("config").Parse(exampleTemplate)
	if err != nil {
		return "", fmt.Errorf("parse config template: %w", err)
	}
	var b strings.Builder
	if err := t.Execute(&b, c); err != nil {
		return "", fmt.Errorf("render config template: %w", err)
	}
	return b.String(), nil
}

// Write writes content to path, creating parent directories as needed. When
// force is false and the file already exists, it returns an error without
// modifying the file.
func Write(path, content string, force bool) error {
	if !force {
		if _, err := os.Stat(path); err == nil {
			return fmt.Errorf("%s already exists (use --force to overwrite)", path)
		} else if !os.IsNotExist(err) {
			return err
		}
	}
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create %s: %w", dir, err)
		}
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}
