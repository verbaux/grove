package main

import (
	"runtime/debug"
	"strings"

	"github.com/verbaux/grove/cmd"
)

func main() {
	cmd.Version = buildVersion()
	cmd.Execute()
}

func buildVersion() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "unknown"
	}

	v := info.Main.Version
	// go install with a tagged module version (e.g. v0.1.0)
	if v != "" && v != "(devel)" && !strings.HasPrefix(v, "v0.0.0-") {
		return v
	}

	// local build: extract commit and dirty flag from vcs settings
	var revision, modified string
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			revision = s.Value
		case "vcs.modified":
			if s.Value == "true" {
				modified = "-dirty"
			}
		}
	}

	if revision != "" {
		if len(revision) > 8 {
			revision = revision[:8]
		}
		return revision + modified
	}

	return "unknown"
}
