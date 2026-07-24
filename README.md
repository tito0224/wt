# wt

A small CLI for managing git worktrees. It lists worktrees, shows whether each
has **uncommitted changes** or **unmerged commits**, and lets you jump into one
by either `cd`-ing your current shell or opening a new terminal tab.

## Features

- List all worktrees with status (`dirty`, `unmerged`, ahead/behind upstream).
- Interactive picker (Bubble Tea) with an action submenu.
- `cd` your current shell into a worktree (via a shell wrapper), or open a new
  terminal tab.
- Pluggable terminal adapters (iTerm2, Terminal.app, tmux, Ghostty, WezTerm).

## Install

```sh
go install github.com/johnlotito/wt/cmd/wt@latest
```

### Shell integration (required for `cd`-in-place)

A child process can't change its parent shell's directory, so `wt` ships a
shell wrapper. Add it to your shell:

```sh
# zsh / bash
eval "$(wt shell zsh)"

# fish
wt shell fish | source
```

Or let `wt` append the line to your rc file for you:

```sh
wt shell install            # detects your shell from $SHELL
wt shell install --shell zsh --dry-run   # preview without writing
```

Restart your shell afterwards.

## Usage

```sh
wt                      # interactive picker (see keys below)
wt list                 # print worktrees + status (current marked with *)
wt create <name>        # create a new worktree + branch (new/add aliases)
wt create <name> --tab  # ...and open a new terminal tab in it
wt open <branch|path>   # print the worktree path (wrapper cd's into it)
wt open <branch> --tab  # open a new terminal tab there instead
wt status [branch]      # detailed status for one or all worktrees
wt remove <branch>      # remove a worktree (rm alias; -f to force)
wt terminals            # list terminal adapters and which is detected
wt config init          # create a config file interactively
wt config path          # print the global config file path
wt config show          # print the effective (layered) configuration
```

Use `-C <dir>` to run as if started in another directory.

### Creation defaults (config)

`wt create` reads defaults for the base branch, target directory, and branch
naming. Configuration is layered, later sources overriding earlier ones:

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
# Supports the {repo} placeholder. The worktree name is appended.
# Default: "../<repo>-worktrees"
dir = "../{repo}-worktrees"

# Branch name = prefix + separator + name (e.g. "jl" + "/" + "foo" -> jl/foo).
branch_prefix = "jl"
branch_separator = "/"
```

Environment equivalents: `WT_BASE_BRANCH`, `WT_DIR`, `WT_BRANCH_PREFIX`,
`WT_BRANCH_SEPARATOR`.

### Interactive keys

Normal mode:

| Key | Action |
|-----|--------|
| `j` / `k` / arrows | move |
| `g` / `G` | jump to top / bottom |
| `enter` | cd current shell into worktree |
| `o` | open in new terminal tab |
| `a` / `tab` | action submenu |
| `r` | remove (asks for confirmation) |
| `/` | search / filter |
| `q` / `esc` | quit (`esc` first clears an active filter) |

Search mode (after `/`): type to filter; `enter` or `esc` returns to normal
mode keeping the filter. Matching uses a subsequence fuzzy match against the
branch and path. When filtering, results are ranked best-match-first (favoring
contiguous, early, and shorter matches); with no filter the list keeps git's
native order (repo root first).

## Terminal selection

The terminal adapter is auto-detected from `$TERM_PROGRAM` / `$TMUX`. Override
with `--terminal <name>` or the `WT_TERMINAL` env var. See `wt terminals`.

## Adding a terminal adapter

Implement `terminal.Adapter` and register it in an `init()`:

```go
type myterm struct{}

func (myterm) Name() string        { return "myterm" }
func (myterm) Detect() bool        { return os.Getenv("TERM_PROGRAM") == "MyTerm" }
func (myterm) OpenTab(dir string) error { /* spawn a tab in dir */ }

func init() { terminal.Register(myterm{}) }
```

## Status meaning

- **dirty** — uncommitted changes (staged, unstaged, or untracked).
- **unmerged** — the branch has commits not in the repo's default branch
  (`origin/HEAD`, else `main`/`master`).
- **↑N / ↓N** — commits ahead of / behind the branch's upstream.
