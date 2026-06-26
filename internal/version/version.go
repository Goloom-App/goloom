// Package version exposes the goloom release version.
package version

import "runtime/debug"

// Version is the release version. It is set at build time via
//
//	-ldflags "-X git.f4mily.net/goloom/internal/version.Version=v1.2.3"
//
// (see the Makefile and Dockerfile). When it is left empty — e.g. a plain
// `go build`, `go run` or `go install` — String falls back to the VCS revision
// recorded in the build info, and finally to "dev".
var Version = ""

// String returns the resolved version string, never empty.
func String() string {
	if Version != "" {
		return Version
	}
	if info, ok := debug.ReadBuildInfo(); ok {
		if v := info.Main.Version; v != "" && v != "(devel)" {
			return v
		}
		var rev, dirty string
		for _, s := range info.Settings {
			switch s.Key {
			case "vcs.revision":
				rev = s.Value
			case "vcs.modified":
				if s.Value == "true" {
					dirty = "-dirty"
				}
			}
		}
		if rev != "" {
			if len(rev) > 12 {
				rev = rev[:12]
			}
			return "dev-" + rev + dirty
		}
	}
	return "dev"
}
