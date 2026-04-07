package main

import (
	"runtime/debug"
	"strings"

	"github.com/verbaux/grove/cmd"
)

var version string

func main() {
	if version != "" {
		cmd.Version = version
	} else {
		cmd.Version = versionFromBuildInfo()
	}
	cmd.Execute()
}

func versionFromBuildInfo() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "unknown"
	}

	v := info.Main.Version
	if v != "" && v != "(devel)" && !strings.HasPrefix(v, "v0.0.0-") {
		return v
	}

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
