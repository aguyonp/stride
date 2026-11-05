// Package buildinfo exposes the build identity injected with
// -ldflags "-X stride/internal/buildinfo.Version=3.0.0 -X ...Commit=abc1234 -X ...Date=2026-11-05T20:00:00Z".
package buildinfo

import (
	"runtime"
	"runtime/debug"
	"strings"
)

var (
	Version = "dev"
	Commit  = ""
	Date    = ""
)

type Info struct {
	Version string `json:"version"`
	Commit  string `json:"commit"`
	BuiltAt string `json:"builtAt"`
	Go      string `json:"go"`
	Label   string `json:"label"`
}

func Get() Info {
	i := Info{Version: Version, Commit: Commit, BuiltAt: Date, Go: runtime.Version()}
	if i.Commit == "" || i.BuiltAt == "" {
		if bi, ok := debug.ReadBuildInfo(); ok {
			for _, s := range bi.Settings {
				switch {
				case s.Key == "vcs.revision" && i.Commit == "":
					i.Commit = s.Value
				case s.Key == "vcs.time" && i.BuiltAt == "":
					i.BuiltAt = s.Value
				}
			}
		}
	}
	if len(i.Commit) > 7 {
		i.Commit = i.Commit[:7]
	}
	i.Label = label(i)
	return i
}

func label(i Info) string {
	parts := []string{strings.TrimPrefix(i.Version, "v")}
	if i.Commit != "" {
		parts = append(parts, i.Commit)
	}
	if i.BuiltAt != "" {
		parts = append(parts, i.BuiltAt)
	}
	return strings.Join(parts, " · ")
}
