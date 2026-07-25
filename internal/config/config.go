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
	// used. It supports the {repo} placeholder. The worktree name is appended
	// as a single flat directory (slashes in the name become '-'; see DirName).
	Dir string `toml:"dir"`
	// BranchPrefix is prepended to the worktree name to form the branch name,
	// joined with BranchSeparator (e.g. prefix "jl", separator "/" -> jl/foo).
	BranchPrefix string `toml:"branch_prefix"`
	// BranchSeparator joins the prefix and the name. Defaults to "/".
	BranchSeparator string `toml:"branch_separator"`
	// Run lists post-create hook commands, executed in order inside a newly
	// created worktree (via `sh -c`) — typically environment setup that every
	// fresh worktree needs (copying .env files, installing dependencies).
	// The first failing command stops the sequence; the worktree is kept
	// either way. There is no WT_* env equivalent (lists don't map cleanly
	// to a single variable); configure it in a file. Note that TOML decoding
	// replaces the whole list, so a repo-local run overrides — not appends
	// to — a global one.
	Run []string `toml:"run"`
}

// Launch holds defaults for opening a worktree in an editor/agent.
type Launch struct {
	// Default is the launcher name (e.g. "code", "cursor", "claude") opened by
	// the quick-launch key. When empty (or not available), the quick-launch key
	// uses the single available launcher if exactly one is installed, and
	// otherwise opens the launcher picker.
	Default string `toml:"default"`
	// OpencodeWebURL is the origin (e.g. "http://localhost:4096") of an
	// already-running `opencode web` server. When set, the "opencode-web"
	// launcher opens a deep link into that server for the worktree instead of
	// starting a new server. Empty = start a fresh `opencode web`.
	OpencodeWebURL string `toml:"opencode_web_url"`
}

// Config is the top-level configuration.
type Config struct {
	Create Create `toml:"create"`
	Launch Launch `toml:"launch"`
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
	if v, ok := os.LookupEnv("WT_LAUNCHER"); ok {
		cfg.Launch.Default = v
	}
	if v, ok := os.LookupEnv("WT_OPENCODE_WEB_URL"); ok {
		cfg.Launch.OpencodeWebURL = v
	}
}

// separator returns the configured branch separator, defaulting to "/".
func (c Create) separator() string {
	if c.BranchSeparator == "" {
		return "/"
	}
	return c.BranchSeparator
}

// BareName removes the configured branch prefix and separator from name when
// present: users naturally type the branch name they want ("jl/foo") where a
// bare worktree name ("foo") is expected, and without this the prefix would
// be applied twice ("jl/jl/foo"). A name that is exactly the bare prefix is
// left alone — there is no way to tell whether it was meant as a full branch
// name or a worktree called "jl".
func (c Create) BareName(name string) string {
	if c.BranchPrefix == "" {
		return name
	}
	return strings.TrimPrefix(name, c.BranchPrefix+c.separator())
}

// BranchName composes a branch name from the configured prefix, separator, and
// the given worktree name. A name that already carries the prefix is used
// as-is rather than prefixed again.
func (c Create) BranchName(name string) string {
	if c.BranchPrefix == "" {
		return name
	}
	return c.BranchPrefix + c.separator() + c.BareName(name)
}

// DirName converts a worktree/branch name into the single path component used
// as the worktree's directory name. Branch names may contain path separators
// (a configured prefix like "jl/foo", or an existing branch like "feat/x");
// using them verbatim would nest directories and, worse, let a name like
// "../evil" escape the configured worktree dir entirely. Replacing separators
// with '-' guarantees a flat, single-component name — at which point ".." is
// only dangerous as the *whole* name, so the degenerate results ("", ".",
// "..") are rejected.
//
// The mapping is deliberately minimal (no case folding, no stripping of dots
// inside names like "v1.2"): predictability beats aggressive slugging.
// Collisions are possible (branches "feat/x" and "feat-x" both map to
// "feat-x"); creation then fails loudly with git's "already exists" error
// rather than silently reusing the directory.
func DirName(name string) (string, error) {
	n := strings.TrimSpace(name)
	n = strings.ReplaceAll(n, "/", "-")
	n = strings.ReplaceAll(n, `\`, "-")
	switch n {
	case "", ".", "..":
		return "", fmt.Errorf("invalid worktree name %q", name)
	}
	return n, nil
}

// WorktreeDir resolves the directory a new worktree named `name` should live
// in, given the repository root. It expands the {repo} placeholder, resolves
// relative paths against repoRoot, and appends the worktree name — with any
// configured branch prefix stripped (see BareName) and normalized to a single
// flat path element via DirName — as the final component. Stripping the
// prefix keeps the result identical whether the user typed "foo" or "jl/foo",
// and gives prefixed branches short directory names ("foo", not "jl-foo").
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
	component, err := DirName(c.BareName(name))
	if err != nil {
		// Degenerate names are rejected by ops before anything is created;
		// this path only serves display (the add-screen path preview). Return
		// the parent dir itself rather than joining "." or ".." (which would
		// resolve to an escaped path).
		return dir
	}
	return filepath.Join(dir, component)
}

// Render returns a commented TOML document for the given config, suitable for
// writing to a config file. It expands the embedded example template with the
// config's values, preserving the template's documentation comments.
func Render(cfg Config) (string, error) {
	if cfg.Create.BranchSeparator == "" {
		cfg.Create.BranchSeparator = "/"
	}
	t, err := template.New("config").Parse(exampleTemplate)
	if err != nil {
		return "", fmt.Errorf("parse config template: %w", err)
	}
	var b strings.Builder
	if err := t.Execute(&b, cfg); err != nil {
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
