# Generates docs/demo.gif — run via `make demo` (requires vhs: brew install vhs).
# The hidden prologue builds a scratch repo with worktrees in varied states
# (dirty, commits ahead of the base branch) so the picker has something real
# to show; the visible part drives the actual TUI.
Output docs/demo.gif

Set FontSize 15
Set Width 1100
Set Height 480
Set Padding 12
Set Theme "Catppuccin Mocha"
Set TypingSpeed 50ms

Hide
# Point HOME at a scratch dir: hermetic (no user git/wt config leaks in) and
# the TUI renders paths as a tidy ~/code/... instead of /tmp gibberish.
Type `cd $(mktemp -d) && export HOME=$(pwd -P) XDG_CONFIG_HOME=$(pwd -P)/.config && mkdir -p $HOME/code && cd $HOME/code` Enter
Type "git init -qb main demo && cd demo && git config user.email demo@example.com && git config user.name Demo" Enter
Type "echo '# demo' > README.md && git add . && git commit -qm 'initial commit'" Enter
Type "git worktree add -q -b feat/api ../demo-worktrees/api && date > ../demo-worktrees/api/wip.txt" Enter
Type "git worktree add -q -b fix/login ../demo-worktrees/login" Enter
Type "cd ../demo-worktrees/login && echo fix > login.go && git add . && git commit -qm 'fix login redirect' && cd ../../demo" Enter
Type "git worktree add -q -b docs/site ../demo-worktrees/site" Enter
Type "clear" Enter
Show

Type "wt"
Sleep 500ms
Enter
Sleep 2.5s

# Move through the list; the status bar follows the selection.
Type "j"
Sleep 900ms
Type "j"
Sleep 1.2s

# Open the details screen for the highlighted worktree.
Type "l"
Sleep 3s
Escape
Sleep 1s

# Fuzzy search.
Type "/"
Sleep 400ms
Type "log"
Sleep 1.5s
Enter
Sleep 1s
Escape
Sleep 800ms

# The add screen: new branch with live path preview, or pick an existing one.
Type "a"
Sleep 1.2s
Type "payments"
Sleep 2s
Escape
Sleep 800ms

Type "q"
Sleep 1s
