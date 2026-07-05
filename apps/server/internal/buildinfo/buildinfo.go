// Package buildinfo exposes the binary's version and VCS provenance. Values
// come from runtime/debug.ReadBuildInfo() (populated automatically by `go
// build` from the module version and the .git checkout) with an ldflags
// override for release builds that inject a tag, e.g.
//
//	go build -ldflags "-X …/internal/buildinfo.version=v1.2.3"
package buildinfo

import (
	"log/slog"
	"runtime"
	"runtime/debug"
)

// version is the release version. "dev" for local/unstamped builds; release
// pipelines override it via -ldflags "-X …/buildinfo.version=<tag>".
var version = "dev"

// Info is the resolved build metadata: version plus the VCS provenance the Go
// toolchain stamps into the binary.
type Info struct {
	Version   string // release tag or module version, "dev" when unstamped
	Revision  string // vcs.revision (git commit), "" when unavailable
	Time      string // vcs.time (commit timestamp, RFC3339), "" when unavailable
	Modified  bool   // vcs.modified: built from a dirty working tree
	GoVersion string // toolchain that produced the binary
}

// Get resolves the build metadata once from the embedded debug.BuildInfo.
func Get() Info {
	info := Info{Version: version, GoVersion: runtime.Version()}
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return info
	}
	// Prefer the module version when the ldflags override is absent and the
	// binary was `go install`ed from a tagged module (main is "(devel)" for
	// local builds, which we leave as the "dev" default).
	if info.Version == "dev" && bi.Main.Version != "" && bi.Main.Version != "(devel)" {
		info.Version = bi.Main.Version
	}
	for _, s := range bi.Settings {
		switch s.Key {
		case "vcs.revision":
			info.Revision = s.Value
		case "vcs.time":
			info.Time = s.Value
		case "vcs.modified":
			info.Modified = s.Value == "true"
		}
	}
	return info
}

// LogValue renders the build info as a grouped slog attribute, so a startup
// line like `logger.Info("starting", "build", buildinfo.Get())` stays
// structured in JSON and readable on the console.
func (i Info) LogValue() slog.Value {
	attrs := []slog.Attr{
		slog.String("version", i.Version),
		slog.String("go", i.GoVersion),
	}
	if i.Revision != "" {
		rev := i.Revision
		if len(rev) > 12 {
			rev = rev[:12]
		}
		attrs = append(attrs, slog.String("revision", rev))
	}
	if i.Time != "" {
		attrs = append(attrs, slog.String("time", i.Time))
	}
	if i.Modified {
		attrs = append(attrs, slog.Bool("dirty", true))
	}
	return slog.GroupValue(attrs...)
}
