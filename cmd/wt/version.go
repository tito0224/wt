package main

import "runtime/debug"

// versionString derives the version reported by `wt --version` from the build
// info the Go toolchain embeds automatically — no ldflags or release scripting
// needed. Binaries built by `go install module@vX.Y.Z` report the module
// version; local `go build` binaries (which report the pseudo-version
// "(devel)") fall back to the VCS commit that was compiled, with a "-dirty"
// suffix when the working tree had uncommitted changes.
func versionString() string {
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return "unknown"
	}

	var rev, dirty string
	for _, s := range bi.Settings {
		switch s.Key {
		case "vcs.revision":
			rev = s.Value
		case "vcs.modified":
			if s.Value == "true" {
				dirty = "-dirty"
			}
		}
	}

	if v := bi.Main.Version; v != "" && v != "(devel)" {
		return v
	}
	if rev != "" {
		if len(rev) > 12 {
			rev = rev[:12]
		}
		return rev + dirty
	}
	return "(devel)"
}
