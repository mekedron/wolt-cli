package cli

import (
	"runtime/debug"
	"strings"
)

const (
	devVersion         = "dev"
	goDevelMainVersion = "(devel)"
	vcsRevisionKey     = "vcs.revision"
	vcsModifiedKey     = "vcs.modified"
)

var readBuildInfo = debug.ReadBuildInfo

func resolvedVersion(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed != "" && trimmed != devVersion {
		return trimmed
	}

	info, ok := readBuildInfo()
	if ok && info != nil {
		mainVersion := strings.TrimSpace(info.Main.Version)
		if mainVersion != "" && mainVersion != goDevelMainVersion {
			return mainVersion
		}
		revision, dirty := buildRevision(info.Settings)
		if revision != "" {
			if dirty {
				return revision + "-dirty"
			}
			return revision
		}
	}

	if trimmed != "" {
		return trimmed
	}

	return devVersion
}

func buildRevision(settings []debug.BuildSetting) (string, bool) {
	var revision string
	dirty := false
	for _, setting := range settings {
		switch setting.Key {
		case vcsRevisionKey:
			revision = strings.TrimSpace(setting.Value)
		case vcsModifiedKey:
			dirty = strings.EqualFold(strings.TrimSpace(setting.Value), "true")
		}
	}
	if len(revision) > 12 {
		revision = revision[:12]
	}
	return revision, dirty
}
