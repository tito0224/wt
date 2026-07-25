package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/BurntSushi/toml"
)

func TestBranchName(t *testing.T) {
	tests := []struct {
		name   string
		create Create
		in     string
		want   string
	}{
		{"empty prefix passes through", Create{}, "feature", "feature"},
		{"default separator", Create{BranchPrefix: "jl"}, "feature", "jl/feature"},
		{"custom separator", Create{BranchPrefix: "jl", BranchSeparator: "-"}, "feature", "jl-feature"},
	}
	for _, tt := range tests {
		if got := tt.create.BranchName(tt.in); got != tt.want {
			t.Errorf("%s: BranchName(%q) = %q; want %q", tt.name, tt.in, got, tt.want)
		}
	}
}

func TestWorktreeDir(t *testing.T) {
	repoRoot := filepath.Join(string(filepath.Separator), "home", "me", "src", "myrepo")

	tests := []struct {
		name   string
		create Create
		want   string
	}{
		{
			"default is sibling <repo>-worktrees",
			Create{},
			filepath.Join(string(filepath.Separator), "home", "me", "src", "myrepo-worktrees", "wt1"),
		},
		{
			"{repo} placeholder expands",
			Create{Dir: filepath.Join("..", "{repo}-trees")},
			filepath.Join(string(filepath.Separator), "home", "me", "src", "myrepo-trees", "wt1"),
		},
		{
			"relative dir resolves against repo root",
			Create{Dir: "worktrees"},
			filepath.Join(repoRoot, "worktrees", "wt1"),
		},
		{
			"absolute dir kept as-is",
			Create{Dir: filepath.Join(string(filepath.Separator), "tmp", "trees")},
			filepath.Join(string(filepath.Separator), "tmp", "trees", "wt1"),
		},
	}
	for _, tt := range tests {
		if got := tt.create.WorktreeDir(repoRoot, "wt1"); got != tt.want {
			t.Errorf("%s: WorktreeDir = %q; want %q", tt.name, got, tt.want)
		}
	}
}

func TestDirName(t *testing.T) {
	tests := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{"feat", "feat", false},
		{"feat/x", "feat-x", false},           // slash flattened
		{"feat/team/x", "feat-team-x", false}, // every slash flattened
		{`feat\x`, "feat-x", false},           // backslash too
		{"../../evil", "..-..-evil", false},   // traversal defused into a flat name
		{"v1.2", "v1.2", false},               // inner dots untouched
		{"a..b", "a..b", false},               // ".." inside a component is harmless
		{"  padded  ", "padded", false},       // surrounding space trimmed
		{"", "", true},                        // nothing usable
		{"   ", "", true},                     //
		{".", "", true},                       // would resolve to the parent dir itself
		{"..", "", true},                      // would escape the parent dir
		{"/", "-", false},                     // odd but harmless flat component
	}
	for _, tt := range tests {
		got, err := DirName(tt.in)
		if tt.wantErr {
			if err == nil {
				t.Errorf("DirName(%q) = %q, want error", tt.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("DirName(%q): unexpected error %v", tt.in, err)
			continue
		}
		if got != tt.want {
			t.Errorf("DirName(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

// TestWorktreeDirFlattensName pins the traversal fix: names with separators
// become a single flat component, and degenerate names fall back to the parent
// dir (display-only; ops rejects them before creation) instead of joining "."
// or ".." — which would resolve to an escaped path.
func TestWorktreeDirFlattensName(t *testing.T) {
	repoRoot := filepath.Join(string(filepath.Separator), "home", "me", "src", "myrepo")
	parent := filepath.Join(string(filepath.Separator), "home", "me", "src", "myrepo-worktrees")
	c := Create{}

	if got, want := c.WorktreeDir(repoRoot, "feat/x"), filepath.Join(parent, "feat-x"); got != want {
		t.Errorf("slashed name: WorktreeDir = %q, want %q", got, want)
	}
	if got, want := c.WorktreeDir(repoRoot, "../../evil"), filepath.Join(parent, "..-..-evil"); got != want {
		t.Errorf("traversal name: WorktreeDir = %q, want %q", got, want)
	}
	for _, degenerate := range []string{"", ".", ".."} {
		if got := c.WorktreeDir(repoRoot, degenerate); got != parent {
			t.Errorf("degenerate name %q: WorktreeDir = %q, want parent dir %q", degenerate, got, parent)
		}
	}
}

// TestRenderRoundTrips pins the config template: rendering a config and
// decoding the result must reproduce the values (especially the run list,
// which the template renders element by element).
func TestRenderRoundTrips(t *testing.T) {
	in := Default()
	in.Create.BranchPrefix = "jl"
	in.Create.Run = []string{`cp ../.env .`, `echo "quoted" && npm i`}

	doc, err := Render(in)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	var out Config
	if err := toml.Unmarshal([]byte(doc), &out); err != nil {
		t.Fatalf("rendered config is not valid TOML: %v\n%s", err, doc)
	}
	if !reflect.DeepEqual(out.Create.Run, in.Create.Run) {
		t.Errorf("run round-trip = %q, want %q", out.Create.Run, in.Create.Run)
	}
	if out.Create.BranchPrefix != "jl" {
		t.Errorf("branch_prefix round-trip = %q, want jl", out.Create.BranchPrefix)
	}

	// An empty run list must render as valid TOML too (run = []).
	empty, err := Render(Default())
	if err != nil {
		t.Fatalf("Render(default): %v", err)
	}
	if err := toml.Unmarshal([]byte(empty), &out); err != nil {
		t.Fatalf("rendered default config is not valid TOML: %v\n%s", err, empty)
	}
}

func TestApplyEnv(t *testing.T) {
	t.Setenv("WT_BASE_BRANCH", "develop")
	t.Setenv("WT_DIR", "/tmp/trees")
	t.Setenv("WT_BRANCH_PREFIX", "jl")
	t.Setenv("WT_BRANCH_SEPARATOR", "-")
	t.Setenv("WT_LAUNCHER", "cursor")
	t.Setenv("WT_OPENCODE_WEB_URL", "http://localhost:4096")

	cfg := Default()
	applyEnv(&cfg)

	if cfg.Create.BaseBranch != "develop" {
		t.Errorf("BaseBranch = %q; want develop", cfg.Create.BaseBranch)
	}
	if cfg.Create.Dir != "/tmp/trees" {
		t.Errorf("Dir = %q; want /tmp/trees", cfg.Create.Dir)
	}
	if cfg.Create.BranchPrefix != "jl" {
		t.Errorf("BranchPrefix = %q; want jl", cfg.Create.BranchPrefix)
	}
	if cfg.Create.BranchSeparator != "-" {
		t.Errorf("BranchSeparator = %q; want -", cfg.Create.BranchSeparator)
	}
	if cfg.Launch.Default != "cursor" {
		t.Errorf("Launch.Default = %q; want cursor", cfg.Launch.Default)
	}
	if cfg.Launch.OpencodeWebURL != "http://localhost:4096" {
		t.Errorf("Launch.OpencodeWebURL = %q; want http://localhost:4096", cfg.Launch.OpencodeWebURL)
	}
}

// clearWTEnv guarantees the WT_* overrides are unset for the test's duration
// (t.Setenv records the original value; Unsetenv then removes it).
func clearWTEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{
		"WT_BASE_BRANCH", "WT_DIR", "WT_BRANCH_PREFIX",
		"WT_BRANCH_SEPARATOR", "WT_LAUNCHER", "WT_OPENCODE_WEB_URL",
	} {
		t.Setenv(k, "")
		os.Unsetenv(k)
	}
}

func TestLoadLayering(t *testing.T) {
	clearWTEnv(t)

	// Global config in a private XDG_CONFIG_HOME.
	xdg := t.TempDir()
	if err := os.MkdirAll(filepath.Join(xdg, "wt"), 0o755); err != nil {
		t.Fatal(err)
	}
	global := "" +
		"[create]\n" +
		"base_branch = \"main\"\n" +
		"branch_prefix = \"global\"\n" +
		"run = [\"echo global-a\", \"echo global-b\"]\n" +
		"[launch]\n" +
		"default = \"code\"\n"
	if err := os.WriteFile(filepath.Join(xdg, "wt", "config.toml"), []byte(global), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CONFIG_HOME", xdg)

	// Repo config overriding part of the global file.
	repoRoot := t.TempDir()
	repo := "" +
		"[create]\n" +
		"branch_prefix = \"repo\"\n" +
		"run = [\"echo repo\"]\n"
	if err := os.WriteFile(filepath.Join(repoRoot, ".wt.toml"), []byte(repo), 0o644); err != nil {
		t.Fatal(err)
	}

	// Env overriding both files.
	t.Setenv("WT_LAUNCHER", "cursor")

	cfg, err := Load(repoRoot)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Create.BaseBranch != "main" {
		t.Errorf("BaseBranch = %q; want main (from global config)", cfg.Create.BaseBranch)
	}
	if cfg.Create.BranchPrefix != "repo" {
		t.Errorf("BranchPrefix = %q; want repo (repo file overrides global)", cfg.Create.BranchPrefix)
	}
	if cfg.Launch.Default != "cursor" {
		t.Errorf("Launch.Default = %q; want cursor (env overrides both files)", cfg.Launch.Default)
	}
	// TOML decoding replaces the whole list: the repo run REPLACES the global
	// one rather than appending to it (documented behavior).
	if !reflect.DeepEqual(cfg.Create.Run, []string{"echo repo"}) {
		t.Errorf("Run = %q; want repo file's list to replace the global one", cfg.Create.Run)
	}
	if cfg.Create.BranchSeparator != "/" {
		t.Errorf("BranchSeparator = %q; want built-in default /", cfg.Create.BranchSeparator)
	}
}

func TestLoadMissingFilesNotAnError(t *testing.T) {
	clearWTEnv(t)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir()) // no global config inside

	cfg, err := Load(t.TempDir()) // no .wt.toml inside
	if err != nil {
		t.Fatalf("Load with no config files: %v", err)
	}
	// Config contains a slice (Create.Run) and is not ==-comparable.
	if !reflect.DeepEqual(cfg, Default()) {
		t.Errorf("Load with no config files = %+v; want built-in defaults %+v", cfg, Default())
	}
}
