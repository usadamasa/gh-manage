// Package version resolves the build version from ldflags or runtime/debug.
package version

import (
	"runtime/debug"
	"strings"
)

// Info holds version information.
type Info struct {
	Version string
	Commit  string
	Date    string
}

// DisplayString formats the version for display.
// Commit and date are omitted while they hold their default values.
func (i Info) DisplayString() string {
	var parts []string
	if i.Commit != "none" {
		parts = append(parts, "commit: "+i.Commit)
	}
	if i.Date != "unknown" {
		parts = append(parts, "built: "+i.Date)
	}
	if len(parts) == 0 {
		return i.Version
	}
	return i.Version + " (" + strings.Join(parts, ", ") + ")"
}

// Replaceable in tests.
var readBuildInfo = debug.ReadBuildInfo

// Resolve prefers ldflags values and falls back to runtime/debug.ReadBuildInfo
// when the binary was built without them (for example via go install).
func Resolve(ldVersion, ldCommit, ldDate string) Info {
	if ldVersion != "dev" {
		return Info{Version: ldVersion, Commit: ldCommit, Date: ldDate}
	}

	info := Info{Version: ldVersion, Commit: ldCommit, Date: ldDate}

	bi, ok := readBuildInfo()
	if !ok {
		return info
	}

	if bi.Main.Version != "" && bi.Main.Version != "(devel)" {
		info.Version = bi.Main.Version
	}
	for _, s := range bi.Settings {
		switch s.Key {
		case "vcs.revision":
			info.Commit = s.Value
		case "vcs.time":
			info.Date = s.Value
		}
	}

	return info
}
