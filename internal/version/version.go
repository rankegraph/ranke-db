// package: version / build
// type:    logic
// job:     String — the version this binary reports, from the linker or its own build info
// limits:  reads what the toolchain stamped; no git, no filesystem
package version

import (
	"runtime/debug"
	"strings"
)

// injected is set at link time: -ldflags "-X .../internal/version.injected=v1.2.3".
var injected string

// String reports this build's version, preferring the linker's value, then the module
// version a `go install module@version` stamped, then the revision a build from a
// checkout carries. Every path leaves the binary able to name itself, which is why there
// are three: a bare `go build` gets no linker flag and a released module has no VCS.
func String() string {
	if injected != "" {
		return injected
	}
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return "unknown"
	}
	if v := bi.Main.Version; v != "" && v != "(devel)" {
		return v
	}
	return fromVCS(bi.Settings)
}

// fromVCS renders a checkout build as its short revision, marked when the tree carried
// uncommitted changes — so a binary built over edits never claims to be the commit.
func fromVCS(settings []debug.BuildSetting) string {
	var revision string
	var dirty bool
	for _, s := range settings {
		switch s.Key {
		case "vcs.revision":
			revision = s.Value
		case "vcs.modified":
			dirty = s.Value == "true"
		}
	}
	if revision == "" {
		return "unknown"
	}
	short := revision
	if len(short) > 12 {
		short = short[:12]
	}
	var b strings.Builder
	b.WriteString("dev-")
	b.WriteString(short)
	if dirty {
		b.WriteString("-dirty")
	}
	return b.String()
}
