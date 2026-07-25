<div align="center">

# wt

**An interactive, PR-aware git worktree manager for your terminal.**

[![CI](https://github.com/tito0224/wt/actions/workflows/ci.yml/badge.svg)](https://github.com/tito0224/wt/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/tito0224/wt?include_prereleases&sort=semver)](https://github.com/tito0224/wt/releases)
[![Go Report Card](https://goreportcard.com/badge/github.com/tito0224/wt)](https://goreportcard.com/report/github.com/tito0224/wt)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

<img src="docs/demo.gif" alt="wt demo" width="900">

</div>

Git worktrees are great; juggling them isn't. `wt` gives you a fast picker
that shows each worktree's real state — uncommitted changes, unpushed
commits, whether its PR merged — and one-keystroke actions: `cd` your
current shell into it, open it in a terminal tab, editor, or AI agent,
create, remove, prune.

## Highlights

- **Interactive picker** with fuzzy search, a details screen, and a live
  status bar (last commit + GitHub PR state via the `gh` CLI).
- **`cd` in place** — a small shell wrapper lets `enter` change your current
  shell's directory, not a subshell's.
- **PR-aware status** — knows a squash-merged branch is merged (git alone
  can't tell), and warns before you remove unmerged or dirty work.
- **Create worktrees fast** — new branches (with configured prefix + base),
  existing branches, or remote branches (`origin/foo` → local tracking
  branch), with post-create hooks for env files / dependency installs.
- **Open anywhere** — terminal tabs (iTerm2, Terminal.app, tmux, Ghostty,
  WezTerm) and editors/agents (VS Code, Cursor, Zed, Claude Code, Copilot,
  Codex, Gemini, Opencode). Every adapter is pluggable.
- **Tidy by design** — flat, predictable worktree directories; stale entries
  get an actionable prune banner.

## Install

```sh
# Homebrew
brew install tito0224/tap/wt

# Go
go install github.com/tito0224/wt/cmd/wt@latest
```

Or grab a prebuilt binary from the [releases page](https://github.com/tito0224/wt/releases).

### Shell integration (required for `cd`-in-place)

A child process can't change its parent shell's directory, so `wt` ships a
shell wrapper. Let `wt` install it:

```sh
wt shell install            # detects your shell from $SHELL
wt shell install --shell zsh --dry-run   # preview without writing
```

Or add it to your rc file yourself:

```sh
# zsh / bash
eval "$(wt shell zsh)"

# fish
wt shell fish | source
```

For bash, the installer writes to `~/.bash_profile` on macOS (where bash runs
as a login shell) and to `~/.bashrc` elsewhere. Restart your shell afterwards.

## Usage

```sh
wt                      # interactive picker (see keys below)
wt list                 # print worktrees + status (current marked with *)
wt add <name>           # add a new worktree + branch
wt add <name> --tab     # ...and open a new terminal tab in it
wt add origin/foo       # worktree on a local branch tracking origin/foo
wt open <branch|path>   # print the worktree path (wrapper cd's into it)
wt open <branch> --tab  # open a new terminal tab there instead
wt status [branch]      # detailed status for one or all worktrees
wt remove <branch>      # remove a worktree (rm alias; -f to force)
wt remove <branch> -b   # ...and delete its branch (-B to force if unmerged)
wt prune                # drop stale entries for manually deleted worktrees
wt terminals            # list terminal adapters and which is detected
wt config init          # create a config file interactively
wt config path          # print the global config file path
wt config show          # print the effective (layered) configuration
```

Use `-C <dir>` to run as if started in another directory.

### Interactive keys

| Key | Action |
|-----|--------|
| `j` / `k` / arrows | move |
| `g` / `G` | jump to top / bottom |
| `enter` | cd current shell into worktree |
| `a` | add a worktree (new, existing, or remote branch) |
| `o` | open in new terminal tab |
| `e` | open in the default editor / agent |
| `E` | choose an editor / agent to open |
| `tab` / `→` / `l` | open the details screen |
| `y` | copy the worktree path to the clipboard |
| `r` | remove (`y` worktree · `b` worktree + branch · `n` cancel) |
| `R` | refresh the worktree list and statuses |
| `P` | prune stale worktree entries (a banner appears when any exist) |
| `/` | search / filter |
| `?` | full key reference |
| `q` / `esc` | quit (`esc` first clears an active filter) |

Search mode (after `/`): type to filter; `enter` or `esc` returns to normal
mode keeping the filter. Matching uses a subsequence fuzzy match against the
branch and path. When filtering, results are ranked best-match-first (favoring
contiguous, early, and shorter matches); with no filter the list keeps git's
native order (repo root first).

As you move the highlight, a status bar shows the selected worktree's last
commit (hash, relative date, author, subject) and, when the `gh` CLI is
installed and the repo is on GitHub, its pull request (number, state, URL).
These load in the background and are cached for the session; press `R` to
refresh the list and statuses.

The add screen (`a`) lets you create a worktree for a new branch, an existing
one, or a remote one: type a name to filter the list of branches shown below
the input. The top row always creates a new branch with the configured name;
use `↑`/`↓` to pick an existing branch instead. Branches already checked out
in another worktree are shown dimmed and can't be selected. Remote-tracking
branches without a local counterpart appear as `origin/foo  (remote)`;
picking one creates a local branch tracking it. The list reflects what git
already knows — run `git fetch` to pick up new remote branches.

### Status tags

- **dirty / clean** — whether there are uncommitted changes (staged,
  unstaged, or untracked).
- **↑N / ↓N** — commits ahead of / behind the branch's upstream.
- **↥N** — commits ahead of the repo's default branch (`origin/HEAD`, else
  `main`/`master`). A neutral count — squash/rebase merges rewrite history,
  so the details screen combines it with PR state to report real merge
  status.
- **locked / prunable** — worktree locked via git; or its directory is gone
  and the entry can be pruned (`P`).

## Configuration

`wt add` reads defaults for the base branch, target directory, and branch
naming. The branch name and the on-disk location follow different rules: the
branch keeps its full name, slashes and all (e.g. `jl/foo`), while the
worktree directory is always a single flat folder derived from the name with
slashes replaced by `-` (branch `feat/x` lives in `.../feat-x`, never a
nested `feat/x/`). With a `branch_prefix` configured, the prefix is stripped
from the directory name (branch `jl/foo` lives in `.../foo`), and typing a
name that already carries the prefix won't double it — `wt add foo` and
`wt add jl/foo` are equivalent.

Configuration is layered, later sources overriding earlier ones:

1. built-in defaults
2. global file: `$XDG_CONFIG_HOME/wt/config.toml` (falls back to
   `~/.config/wt/config.toml`)
3. per-repo file: `.wt.toml` at the repository root
4. `WT_*` environment variables
5. command-line flags (`--base`, `--into`)

Run `wt config init` to create a config file. It prompts for each value and
writes to the repo-local `.wt.toml` when inside a git repository, otherwise to
the global file. Use `--global`/`--repo` to choose explicitly, `--yes` to skip
prompts and write a commented template, and `--force` to overwrite.

Example `config.toml` (or repo-local `.wt.toml`):

```toml
[create]
# Branch/ref new worktrees are created from.
# Empty = repo default branch (origin/HEAD, then main/master).
base_branch = ""

# Where worktrees are placed. Relative paths resolve against the repo root.
# Supports the {repo} placeholder. The worktree name is appended as a
# single flat directory (slashes in the name become '-').
# Default: "../<repo>-worktrees"
dir = "../{repo}-worktrees"

# Branch name = prefix + separator + name (e.g. "jl" + "/" + "foo" -> jl/foo).
branch_prefix = "jl"
branch_separator = "/"

# Post-create hooks: commands run (via `sh -c`) inside a newly created
# worktree, in order — e.g. copying env files or installing dependencies.
run = ["cp $WT_REPO_ROOT/.env .", "npm install"]
```

Environment equivalents: `WT_BASE_BRANCH`, `WT_DIR`, `WT_BRANCH_PREFIX`,
`WT_BRANCH_SEPARATOR`, `WT_LAUNCHER` (maps to `launch.default`), and
`WT_OPENCODE_WEB_URL` (maps to `launch.opencode_web_url`). `create.run` has
no env form (lists don't map cleanly to a single variable).

### Post-create hooks

`create.run` commands run inside the new worktree after `wt add` (or the
TUI's add screen) creates it — for new, existing, and remote branches. Each
command's environment includes:

| Variable | Value |
|----------|-------|
| `WT_WORKTREE` | absolute path of the new worktree (also the working directory) |
| `WT_BRANCH` | branch checked out in it (e.g. `jl/foo`) |
| `WT_REPO_ROOT` | absolute path of the repository the worktree was added from |

The first failing command stops the sequence; the worktree is **kept**
either way and the failure is reported. From the CLI, hook output streams
live to stderr.

Note that a repo-local `.wt.toml` can run arbitrary commands via this key —
the same trust model as npm scripts or direnv. Only create worktrees in
repositories you trust.

## Terminal selection

The terminal adapter is auto-detected from `$TERM_PROGRAM` / `$TMUX`. Override
with `--terminal <name>` or the `WT_TERMINAL` env var. See `wt terminals`.

### Platform support

The primary target is macOS: the iTerm2, Terminal.app, and Ghostty adapters
use macOS mechanisms (AppleScript / `open`). The tmux and WezTerm adapters
work cross-platform. On Linux, URLs are opened via `xdg-open`. There is
currently no Windows terminal adapter. Editor/agent launchers are detected
via `PATH` and, on macOS, app bundles.

### Adding a terminal adapter

Implement `terminal.Adapter` and register it in an `init()`:

```go
type myterm struct{}

func (myterm) Name() string { return "myterm" }
func (myterm) Detect() bool { return os.Getenv("TERM_PROGRAM") == "MyTerm" }

// OpenTab spawns a tab in dir. When command is non-empty, the tab runs that
// argv (command[0] with command[1:] as args) instead of an interactive shell.
func (myterm) OpenTab(dir string, command ...string) error {
	// spawn a tab in dir, running command if given
	return nil
}

func init() { terminal.Register(myterm{}) }
```

The same pattern applies to forge adapters (PR lookups; GitHub via `gh` today)
and launcher adapters (editors/agents).

## Development

Checks are plain commands behind Makefile targets; CI and the pre-commit
hook run the same targets.

```sh
make hooks   # one-time: install the pre-commit hook
make check   # everything CI runs (build, fmt, vet, staticcheck, errcheck,
             # tidy drift, govulncheck, race tests)
make demo    # regenerate docs/demo.gif (requires vhs)
```

Releases are cut by pushing a semver tag: CI gates the tag, builds
cross-platform artifacts, drafts release notes, publishes a GitHub Release,
and bumps the Homebrew tap.

## License

[MIT](LICENSE)
