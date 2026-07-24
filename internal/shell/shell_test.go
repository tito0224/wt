package shell

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestWrapper(t *testing.T) {
	for _, sh := range []string{"zsh", "bash", "fish"} {
		src, err := Wrapper(sh)
		if err != nil {
			t.Errorf("Wrapper(%q): %v", sh, err)
			continue
		}
		if !strings.Contains(src, "wt") {
			t.Errorf("Wrapper(%q) output does not define a wt function:\n%s", sh, src)
		}
	}
	if _, err := Wrapper("csh"); err == nil {
		t.Error("Wrapper(csh) succeeded; want unsupported-shell error")
	}
}

func TestInstall(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	res, err := Install("zsh", false)
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	if !res.Wrote || res.AlreadyExists {
		t.Errorf("first Install = {Wrote:%t AlreadyExists:%t}; want {true false}", res.Wrote, res.AlreadyExists)
	}
	data, err := os.ReadFile(res.RCFile)
	if err != nil {
		t.Fatalf("read rc file: %v", err)
	}
	if !strings.Contains(string(data), marker) || !strings.Contains(string(data), markerEnd) {
		t.Errorf("rc file missing marker block:\n%s", data)
	}

	// Second install must detect the existing block and change nothing.
	res2, err := Install("zsh", false)
	if err != nil {
		t.Fatalf("second Install: %v", err)
	}
	if !res2.AlreadyExists || res2.Wrote {
		t.Errorf("second Install = {Wrote:%t AlreadyExists:%t}; want {false true}", res2.Wrote, res2.AlreadyExists)
	}
}

func TestInstallDryRun(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	res, err := Install("zsh", true)
	if err != nil {
		t.Fatalf("Install dry-run: %v", err)
	}
	if res.Wrote {
		t.Error("dry-run reported Wrote=true")
	}
	if _, err := os.Stat(res.RCFile); !os.IsNotExist(err) {
		t.Errorf("dry-run created %s (stat err = %v); want no file", res.RCFile, err)
	}
}

func TestInstallUnsupported(t *testing.T) {
	if _, err := Install("csh", true); err == nil {
		t.Error("Install(csh) succeeded; want unsupported-shell error")
	}
}

func TestRCFileBash(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	rc, err := rcFile("bash")
	if err != nil {
		t.Fatalf("rcFile(bash): %v", err)
	}
	// macOS terminals run bash as a login shell (reads ~/.bash_profile);
	// elsewhere interactive shells read ~/.bashrc.
	want := filepath.Join(home, ".bashrc")
	if runtime.GOOS == "darwin" {
		want = filepath.Join(home, ".bash_profile")
	}
	if rc != want {
		t.Errorf("rcFile(bash) = %q; want %q on %s", rc, want, runtime.GOOS)
	}
}
