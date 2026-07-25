package main

import "runtime/debug"

// version is injected by release builds via
// `-ldflags "-X main.version=v1.2.3"` (see `make dist`): cross-compiled
// artifacts are built from a tag in CI, where the explicit tag is more
// reliable than VCS stamping. Empty for normal go build/install builds,
// which use the toolchain's embedded build info instead.
var version string

// versionString derives the version reported by `wt --version`. Release
// artifacts carry an injected version; otherwise it comes from the build info
// the Go toolchain embeds automatically — binaries built by
// `go install module@vX.Y.Z` report the module version, and local `go build`
// binaries report the VCS-derived pseudo-version, with a "-dirty" suffix when
// the working tree had uncommitted changes.
func versionString() string {
	if version != "" {
		return version
	}
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
