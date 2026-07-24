package terminal

import (
	"os"
	"strings"
	"testing"
)

func TestShellQuote(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"simple", "'simple'"},
		{"/path/with spaces", "'/path/with spaces'"},
		// An embedded single quote must be closed, escaped, and reopened.
		{"it's", `'it'\''s'`},
	}
	for _, tt := range tests {
		if got := shellQuote(tt.in); got != tt.want {
			t.Errorf("shellQuote(%q) = %s; want %s", tt.in, got, tt.want)
		}
	}
}

func TestCdCommandLine(t *testing.T) {
	if got, want := cdCommandLine("/tmp/dir", nil), "cd '/tmp/dir'"; got != want {
		t.Errorf("cdCommandLine no command = %q; want %q", got, want)
	}
	got := cdCommandLine("/tmp/my dir", []string{"claude", "--flag", "a b"})
	want := "cd '/tmp/my dir' && 'claude' '--flag' 'a b'"
	if got != want {
		t.Errorf("cdCommandLine with command = %q; want %q", got, want)
	}
}

// unsetenv clears name for the test's duration. t.Setenv is used first so the
// original value is restored on cleanup.
func unsetenv(t *testing.T, name string) {
	t.Helper()
	t.Setenv(name, "")
	os.Unsetenv(name)
}

func TestResolveExplicitUnknown(t *testing.T) {
	_, err := Resolve("no-such-terminal")
	if err == nil {
		t.Fatal("Resolve(unknown) succeeded; want error")
	}
	if !strings.Contains(err.Error(), "no-such-terminal") {
		t.Errorf("error %q does not mention the requested name", err)
	}
}

func TestResolveWTTerminalEnv(t *testing.T) {
	t.Setenv("WT_TERMINAL", "tmux")
	a, err := Resolve("")
	if err != nil {
		t.Fatalf("Resolve with WT_TERMINAL=tmux: %v", err)
	}
	if a.Name() != "tmux" {
		t.Errorf("Resolve returned %q; want tmux", a.Name())
	}
}

func TestResolveWTTerminalEnvUnknown(t *testing.T) {
	t.Setenv("WT_TERMINAL", "bogus")
	_, err := Resolve("")
	if err == nil {
		t.Fatal("Resolve with unknown WT_TERMINAL succeeded; want error")
	}
	if !strings.Contains(err.Error(), "WT_TERMINAL") {
		t.Errorf("error %q does not mention WT_TERMINAL", err)
	}
}

// TestResolveDetectPrefersTmux pins the deterministic auto-detection order:
// running tmux inside iTerm2 makes both adapters detect, and tmux (the
// innermost environment) must win.
func TestResolveDetectPrefersTmux(t *testing.T) {
	unsetenv(t, "WT_TERMINAL")
	t.Setenv("TMUX", "/tmp/x")
	t.Setenv("TERM_PROGRAM", "iTerm.app")

	a, err := Resolve("")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if a.Name() != "tmux" {
		t.Errorf("Resolve detected %q; want tmux to win over iterm", a.Name())
	}
}
