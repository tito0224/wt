package git

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Pure unit tests (no git binary required).
// ---------------------------------------------------------------------------

func TestParsePorcelain(t *testing.T) {
	// One blob covering every field parsePorcelain understands: a bare entry,
	// a normal branch entry (refs/heads/ must be stripped) that is locked, and
	// a detached entry that is prunable.
	out := strings.Join([]string{
		"worktree /repo/.bare",
		"bare",
		"",
		"worktree /repo/main",
		"HEAD 1111111111111111111111111111111111111111",
		"branch refs/heads/main",
		"locked",
		"",
		"worktree /repo/det",
		"HEAD 2222222222222222222222222222222222222222",
		"detached",
		"prunable",
		"",
	}, "\n")

	wts := parsePorcelain(out)
	if len(wts) != 3 {
		t.Fatalf("got %d worktrees, want 3: %+v", len(wts), wts)
	}

	bare := wts[0]
	if bare.Path != "/repo/.bare" || !bare.Bare {
		t.Errorf("bare entry = %+v, want Path=/repo/.bare Bare=true", bare)
	}

	main := wts[1]
	if main.Path != "/repo/main" {
		t.Errorf("main.Path = %q", main.Path)
	}
	if main.Head != "1111111111111111111111111111111111111111" {
		t.Errorf("main.Head = %q", main.Head)
	}
	if main.Branch != "main" {
		t.Errorf("main.Branch = %q, want refs/heads/ stripped to %q", main.Branch, "main")
	}
	if !main.Locked {
		t.Error("main.Locked = false, want true")
	}
	if main.Detached || main.Bare || main.Prunable {
		t.Errorf("main has unexpected flags: %+v", main)
	}

	det := wts[2]
	if !det.Detached || det.Branch != "" {
		t.Errorf("detached entry = %+v, want Detached=true Branch=\"\"", det)
	}
	if !det.Prunable {
		t.Error("det.Prunable = false, want true")
	}
}

func TestParsePorcelainEmpty(t *testing.T) {
	if wts := parsePorcelain(""); len(wts) != 0 {
		t.Fatalf("parsePorcelain(\"\") = %+v, want empty", wts)
	}
}

func TestStripRemote(t *testing.T) {
	tests := []struct{ in, want string }{
		{"origin/main", "main"},
		{"main", "main"},
		// Documents current behavior: only the first path segment is treated
		// as the remote name, so a slash inside the branch name survives.
		{"origin/feat/x", "feat/x"},
	}
	for _, tt := range tests {
		if got := stripRemote(tt.in); got != tt.want {
			t.Errorf("stripRemote(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestParseCommit(t *testing.T) {
	c, ok := parseCommit("a1b2c3d\x00Ada Lovelace\x003 days ago\x00fix: things")
	if !ok {
		t.Fatal("parseCommit returned ok=false for a valid record")
	}
	want := Commit{ShortHash: "a1b2c3d", Author: "Ada Lovelace", RelDate: "3 days ago", Subject: "fix: things"}
	if c != want {
		t.Errorf("parseCommit = %+v, want %+v", c, want)
	}

	if _, ok := parseCommit("a1b2c3d\x00only two"); ok {
		t.Error("parseCommit accepted a record with too few fields")
	}
}

// ---------------------------------------------------------------------------
// Integration tests against a real git binary in hermetic temp repositories.
// ---------------------------------------------------------------------------

// newTestRepo creates a hermetic git repository with a deterministic "main"
// default branch and one initial commit, and returns its path. Global and
// system git config are neutralized via environment so the test never picks
// up the developer's settings (hooks, quotepath, signing, default branch...).
func newTestRepo(t *testing.T) string {
	t.Helper()
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_SYSTEM", os.DevNull)
	scrubGitEnv(t)

	dir := filepath.Join(t.TempDir(), "repo")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	mustGit(t, dir, "init", "-b", "main")
	mustGit(t, dir, "config", "user.name", "wt test")
	mustGit(t, dir, "config", "user.email", "wt@example.invalid")
	commitFile(t, dir, "README.md", "hello\n", "initial commit")
	return dir
}

// scrubGitEnv removes git's repository-context environment variables. Git
// hooks (and tools like `git rebase --exec`) export GIT_DIR & co. pointing at
// the surrounding repository — often as a relative path like ".git" — which
// would leak into every git subprocess these tests (and the code under test)
// spawn inside temp repos, breaking them in confusing ways. t.Setenv registers
// restoration of the original values; the follow-up Unsetenv removes them for
// the test's duration.
func scrubGitEnv(t *testing.T) {
	t.Helper()
	for _, v := range []string{
		"GIT_DIR", "GIT_WORK_TREE", "GIT_INDEX_FILE",
		"GIT_COMMON_DIR", "GIT_OBJECT_DIRECTORY", "GIT_PREFIX",
	} {
		t.Setenv(v, "") // register cleanup that restores the original value
		os.Unsetenv(v)
	}
}

// mustGit runs a git command in dir and fails the test on error.
func mustGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	return mustGitEnv(t, dir, nil, args...)
}

// mustGitEnv is mustGit with extra environment variables appended, used to
// control commit metadata such as GIT_COMMITTER_DATE deterministically.
func mustGitEnv(t *testing.T, dir string, extraEnv []string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), extraEnv...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

// commitFile writes content to name (relative to dir), stages it, and commits
// with the given message.
func commitFile(t *testing.T, dir, name, content, msg string) {
	t.Helper()
	commitFileAt(t, dir, name, content, msg, "")
}

// commitFileAt is commitFile with an optional RFC3339 committer/author date,
// letting tests control the timestamps CommitsAfter filters on.
func commitFileAt(t *testing.T, dir, name, content, msg, date string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	mustGit(t, dir, "add", "--", name)
	var env []string
	if date != "" {
		env = []string{"GIT_COMMITTER_DATE=" + date, "GIT_AUTHOR_DATE=" + date}
	}
	mustGitEnv(t, dir, env, "commit", "-m", msg)
}

// findWorktree returns the worktree whose canonical path matches path, failing
// the test when absent. Canonical comparison matters on macOS, where TempDir
// paths go through the /var -> /private/var symlink.
func findWorktree(t *testing.T, wts []Worktree, path string) Worktree {
	t.Helper()
	want := canonical(path)
	for _, w := range wts {
		if canonical(w.Path) == want {
			return w
		}
	}
	t.Fatalf("no worktree with path %q in %+v", path, wts)
	return Worktree{}
}

func TestStatus(t *testing.T) {
	t.Run("clean", func(t *testing.T) {
		dir := newTestRepo(t)
		s, err := Status(dir)
		if err != nil {
			t.Fatal(err)
		}
		if !s.Clean() || s.Staged != 0 || s.Modified != 0 || s.Untracked != 0 {
			t.Errorf("Status = %+v, want clean", s)
		}
	})

	t.Run("untracked", func(t *testing.T) {
		dir := newTestRepo(t)
		if err := os.WriteFile(filepath.Join(dir, "new.txt"), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		s, err := Status(dir)
		if err != nil {
			t.Fatal(err)
		}
		if s.Untracked != 1 || s.Staged != 0 || s.Modified != 0 {
			t.Errorf("Status = %+v, want exactly 1 untracked", s)
		}
		if len(s.Files) != 1 || s.Files[0].Code != "??" || s.Files[0].Path != "new.txt" {
			t.Errorf("Files = %+v, want [{?? new.txt}]", s.Files)
		}
	})

	t.Run("modified tracked", func(t *testing.T) {
		dir := newTestRepo(t)
		if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("changed\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		s, err := Status(dir)
		if err != nil {
			t.Fatal(err)
		}
		if s.Modified != 1 || s.Staged != 0 || s.Untracked != 0 {
			t.Errorf("Status = %+v, want exactly 1 modified", s)
		}
		if len(s.Files) != 1 || s.Files[0].Code != " M" || s.Files[0].Path != "README.md" {
			t.Errorf("Files = %+v, want [{\" M\" README.md}]", s.Files)
		}
	})

	t.Run("staged", func(t *testing.T) {
		dir := newTestRepo(t)
		if err := os.WriteFile(filepath.Join(dir, "new.txt"), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		mustGit(t, dir, "add", "new.txt")
		s, err := Status(dir)
		if err != nil {
			t.Fatal(err)
		}
		if s.Staged != 1 || s.Modified != 0 || s.Untracked != 0 {
			t.Errorf("Status = %+v, want exactly 1 staged", s)
		}
		if len(s.Files) != 1 || s.Files[0].Code != "A " || s.Files[0].Path != "new.txt" {
			t.Errorf("Files = %+v, want [{\"A \" new.txt}]", s.Files)
		}
	})

	t.Run("staged rename", func(t *testing.T) {
		dir := newTestRepo(t)
		mustGit(t, dir, "mv", "README.md", "RENAMED.md")
		s, err := Status(dir)
		if err != nil {
			t.Fatal(err)
		}
		if s.Staged != 1 || s.Modified != 0 || s.Untracked != 0 {
			t.Errorf("Status = %+v, want exactly 1 staged", s)
		}
		if len(s.Files) != 1 {
			t.Fatalf("Files = %+v, want exactly 1 entry", s.Files)
		}
		f := s.Files[0]
		if f.Code != "R " {
			t.Errorf("Code = %q, want %q", f.Code, "R ")
		}
		if f.Path != "README.md -> RENAMED.md" {
			t.Errorf("Path = %q, want %q", f.Path, "README.md -> RENAMED.md")
		}
	})

	t.Run("path with space is not quoted", func(t *testing.T) {
		dir := newTestRepo(t)
		if err := os.WriteFile(filepath.Join(dir, "has space.txt"), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		s, err := Status(dir)
		if err != nil {
			t.Fatal(err)
		}
		if len(s.Files) != 1 || s.Files[0].Path != "has space.txt" {
			t.Errorf("Files = %+v, want path %q verbatim (no quotes)", s.Files, "has space.txt")
		}
	})
}

func TestListWorktrees(t *testing.T) {
	dir := newTestRepo(t)
	wtPath := filepath.Join(filepath.Dir(dir), "feat-wt")
	mustGit(t, dir, "worktree", "add", "-b", "feat", wtPath)

	wts, err := ListWorktrees(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(wts) != 2 {
		t.Fatalf("got %d worktrees, want 2: %+v", len(wts), wts)
	}

	main := findWorktree(t, wts, dir)
	if main.Branch != "main" {
		t.Errorf("main.Branch = %q, want %q", main.Branch, "main")
	}
	if !main.Current {
		t.Error("main.Current = false, want true (listed from the repo dir)")
	}
	if main.Dirty {
		t.Error("main.Dirty = true, want false")
	}

	feat := findWorktree(t, wts, wtPath)
	if feat.Branch != "feat" {
		t.Errorf("feat.Branch = %q, want %q", feat.Branch, "feat")
	}
	if feat.Current {
		t.Error("feat.Current = true, want false")
	}
	if feat.Dirty {
		t.Error("feat.Dirty = true, want false")
	}

	// Dirtying the added worktree must flip only its Dirty flag.
	if err := os.WriteFile(filepath.Join(wtPath, "scratch.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	wts, err = ListWorktrees(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !findWorktree(t, wts, wtPath).Dirty {
		t.Error("feat.Dirty = false after adding a file, want true")
	}
	if findWorktree(t, wts, dir).Dirty {
		t.Error("main.Dirty = true, want false")
	}
}

func TestAddAddExistingRemove(t *testing.T) {
	dir := newTestRepo(t)

	// Add must create the (missing) parent directory, the branch, and the
	// worktree in one go.
	newPath := filepath.Join(filepath.Dir(dir), "nested", "deeper", "feat-wt")
	if err := Add(dir, newPath, "feature", ""); err != nil {
		t.Fatalf("Add: %v", err)
	}
	wts, err := ListWorktrees(dir)
	if err != nil {
		t.Fatal(err)
	}
	if w := findWorktree(t, wts, newPath); w.Branch != "feature" {
		t.Errorf("added worktree branch = %q, want %q", w.Branch, "feature")
	}

	// AddExisting checks out a branch that already exists without creating one.
	mustGit(t, dir, "branch", "existing")
	existingPath := filepath.Join(filepath.Dir(dir), "existing-wt")
	if err := AddExisting(dir, existingPath, "existing"); err != nil {
		t.Fatalf("AddExisting: %v", err)
	}
	wts, err = ListWorktrees(dir)
	if err != nil {
		t.Fatal(err)
	}
	if w := findWorktree(t, wts, existingPath); w.Branch != "existing" {
		t.Errorf("existing worktree branch = %q, want %q", w.Branch, "existing")
	}

	// Remove both; the list must shrink back to just the main worktree.
	if err := Remove(dir, newPath, false); err != nil {
		t.Fatalf("Remove(%s): %v", newPath, err)
	}
	if err := Remove(dir, existingPath, false); err != nil {
		t.Fatalf("Remove(%s): %v", existingPath, err)
	}
	wts, err = ListWorktrees(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(wts) != 1 {
		t.Fatalf("after Remove, got %d worktrees, want 1: %+v", len(wts), wts)
	}
}

func TestDeleteBranch(t *testing.T) {
	dir := newTestRepo(t)

	// A branch pointing at main's HEAD is fully merged: safe delete succeeds.
	mustGit(t, dir, "branch", "merged")
	if err := DeleteBranch(dir, "merged", false); err != nil {
		t.Fatalf("DeleteBranch(merged, safe): %v", err)
	}

	// A branch with a commit not on main is unmerged: safe delete must fail
	// with an error detectable via errors.Is, and force delete must succeed.
	mustGit(t, dir, "switch", "-c", "unmerged")
	commitFile(t, dir, "extra.txt", "x\n", "unmerged work")
	mustGit(t, dir, "switch", "main")

	err := DeleteBranch(dir, "unmerged", false)
	if err == nil {
		t.Fatal("DeleteBranch(unmerged, safe) succeeded, want error")
	}
	if !errors.Is(err, ErrBranchNotMerged) {
		t.Fatalf("DeleteBranch(unmerged, safe) = %v, want errors.Is(err, ErrBranchNotMerged)", err)
	}
	if err := DeleteBranch(dir, "unmerged", true); err != nil {
		t.Fatalf("DeleteBranch(unmerged, force): %v", err)
	}
}

// addTestRemote registers a bare clone of dir as "origin", pushes all local
// branches, creates one remote-only branch, and fetches so remote-tracking
// refs exist. It also sets origin/HEAD so the HEAD-exclusion path is covered.
func addTestRemote(t *testing.T, dir string) {
	t.Helper()
	bare := filepath.Join(t.TempDir(), "origin.git")
	mustGit(t, dir, "clone", "--bare", dir, bare)
	mustGit(t, dir, "remote", "add", "origin", bare)
	// A branch that exists only on the remote.
	mustGit(t, dir, "branch", "remote-only")
	mustGit(t, dir, "push", "-q", "origin", "--all")
	mustGit(t, dir, "branch", "-D", "remote-only")
	mustGit(t, dir, "fetch", "-q", "origin")
	mustGit(t, dir, "remote", "set-head", "origin", "--auto")
}

func TestBranches(t *testing.T) {
	dir := newTestRepo(t)
	mustGit(t, dir, "branch", "idle")
	wtPath := filepath.Join(filepath.Dir(dir), "feat-wt")
	mustGit(t, dir, "worktree", "add", "-b", "feat", wtPath)
	addTestRemote(t, dir)

	branches, err := Branches(dir)
	if err != nil {
		t.Fatal(err)
	}
	byFull := make(map[string]Branch, len(branches))
	for _, b := range branches {
		byFull[b.FullName()] = b
	}
	for _, name := range []string{"main", "idle", "feat", "origin/main", "origin/remote-only"} {
		if _, ok := byFull[name]; !ok {
			t.Errorf("branch %q missing from %+v", name, branches)
		}
	}
	if _, ok := byFull["origin/HEAD"]; ok {
		t.Error("origin/HEAD must be excluded")
	}

	// Locals first, remotes after (stable partition).
	sawRemote := false
	for _, b := range branches {
		if b.IsRemote() {
			sawRemote = true
		} else if sawRemote {
			t.Fatalf("local branch %q listed after a remote one: %+v", b.Name, branches)
		}
	}

	if b := byFull["origin/remote-only"]; b.Remote != "origin" || b.Name != "remote-only" {
		t.Errorf("remote-only = %+v, want Remote=origin Name=remote-only", b)
	}
	if b := byFull["feat"]; canonical(b.CheckedOutAt) != canonical(wtPath) {
		t.Errorf("feat.CheckedOutAt = %q, want %q", b.CheckedOutAt, wtPath)
	} else if !b.InUse() {
		t.Error("feat.InUse() = false, want true")
	}
	if b := byFull["idle"]; b.InUse() {
		t.Errorf("idle.InUse() = true (CheckedOutAt=%q), want false", b.CheckedOutAt)
	}
	if b := byFull["main"]; !b.InUse() {
		t.Error("main.InUse() = false, want true (checked out in the main worktree)")
	}
}

func TestAddTracking(t *testing.T) {
	dir := newTestRepo(t)
	addTestRemote(t, dir)

	path := filepath.Join(filepath.Dir(dir), "tracked-wt")
	if err := AddTracking(dir, path, "remote-only", "origin/remote-only"); err != nil {
		t.Fatal(err)
	}
	if got := mustGit(t, path, "rev-parse", "--abbrev-ref", "HEAD"); got != "remote-only" {
		t.Errorf("checked-out branch = %q, want remote-only", got)
	}
	if got := mustGit(t, path, "rev-parse", "--abbrev-ref", "@{upstream}"); got != "origin/remote-only" {
		t.Errorf("upstream = %q, want origin/remote-only", got)
	}
}

// TestPrune pins the prunable round-trip: a manually deleted worktree dir is
// flagged Prunable by ListWorktrees, and Prune drops the stale entry.
func TestPrune(t *testing.T) {
	dir := newTestRepo(t)
	wtPath := filepath.Join(filepath.Dir(dir), "gone-wt")
	mustGit(t, dir, "worktree", "add", "-b", "gone", wtPath)
	if err := os.RemoveAll(wtPath); err != nil {
		t.Fatal(err)
	}

	wts, err := ListWorktrees(dir)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, w := range wts {
		if w.Branch == "gone" {
			found = true
			if !w.Prunable {
				t.Error("deleted worktree not flagged Prunable")
			}
		}
	}
	if !found {
		t.Fatalf("stale worktree missing from list: %+v", wts)
	}

	if err := Prune(dir); err != nil {
		t.Fatal(err)
	}
	wts, err = ListWorktrees(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, w := range wts {
		if w.Branch == "gone" {
			t.Fatalf("stale entry survived Prune: %+v", w)
		}
	}
}

func TestDefaultBranch(t *testing.T) {
	dir := newTestRepo(t)
	// No remotes exist, so the remote-HEAD probe must fall through to the
	// local "main" branch.
	if got := DefaultBranch(dir); got != "main" {
		t.Errorf("DefaultBranch = %q, want %q", got, "main")
	}
}

func TestRecentCommits(t *testing.T) {
	dir := newTestRepo(t)
	commitFile(t, dir, "a.txt", "a\n", "second commit")
	commitFile(t, dir, "b.txt", "b\n", "third commit")

	commits, err := RecentCommits(dir, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(commits) != 2 {
		t.Fatalf("got %d commits, want 2: %+v", len(commits), commits)
	}
	if commits[0].Subject != "third commit" || commits[1].Subject != "second commit" {
		t.Errorf("subjects = [%q, %q], want most-recent-first [%q, %q]",
			commits[0].Subject, commits[1].Subject, "third commit", "second commit")
	}
	for i, c := range commits {
		if c.ShortHash == "" || c.Author == "" || c.RelDate == "" {
			t.Errorf("commits[%d] has empty fields: %+v", i, c)
		}
	}
}

func TestCommitsAfter(t *testing.T) {
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_SYSTEM", os.DevNull)

	dir := filepath.Join(t.TempDir(), "repo")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	mustGit(t, dir, "init", "-b", "main")
	mustGit(t, dir, "config", "user.name", "wt test")
	mustGit(t, dir, "config", "user.email", "wt@example.invalid")

	// Deterministic committer dates so the --since filter has fixed anchors:
	// main's commit in January, the feature commit in June.
	commitFileAt(t, dir, "base.txt", "b\n", "base commit", "2024-01-01T12:00:00Z")
	mustGit(t, dir, "switch", "-c", "feat")
	commitFileAt(t, dir, "feat.txt", "f\n", "feat commit", "2024-06-01T12:00:00Z")

	// Empty since is a documented no-op regardless of history.
	if n, err := CommitsAfter(dir, "main", ""); err != nil || n != 0 {
		t.Errorf("CommitsAfter(empty since) = %d, %v; want 0, nil", n, err)
	}

	// Since before the feature commit: the one commit on feat but not main.
	if n, err := CommitsAfter(dir, "main", "2024-03-01T00:00:00Z"); err != nil || n != 1 {
		t.Errorf("CommitsAfter(since March) = %d, %v; want 1, nil", n, err)
	}

	// Since after every commit: nothing qualifies.
	if n, err := CommitsAfter(dir, "main", "2024-12-01T00:00:00Z"); err != nil || n != 0 {
		t.Errorf("CommitsAfter(since December) = %d, %v; want 0, nil", n, err)
	}

	// Without a base ref, all of HEAD's history is eligible for the filter.
	if n, err := CommitsAfter(dir, "", "2023-01-01T00:00:00Z"); err != nil || n != 2 {
		t.Errorf("CommitsAfter(no base) = %d, %v; want 2, nil", n, err)
	}
}
