package cli

import (
	"runtime/debug"
	"testing"
)

func TestResolvedVersionPrefersInjectedVersion(t *testing.T) {
	orig := readBuildInfo
	t.Cleanup(func() {
		readBuildInfo = orig
	})
	readBuildInfo = func() (*debug.BuildInfo, bool) {
		return &debug.BuildInfo{Main: debug.Module{Version: "v9.9.9"}}, true
	}

	got := resolvedVersion("v1.2.3")
	if got != "v1.2.3" {
		t.Fatalf("expected injected version v1.2.3, got %q", got)
	}
}

func TestResolvedVersionUsesBuildInfoModuleVersion(t *testing.T) {
	orig := readBuildInfo
	t.Cleanup(func() {
		readBuildInfo = orig
	})
	readBuildInfo = func() (*debug.BuildInfo, bool) {
		return &debug.BuildInfo{Main: debug.Module{Version: "v0.4.0"}}, true
	}

	got := resolvedVersion(devVersion)
	if got != "v0.4.0" {
		t.Fatalf("expected build info module version v0.4.0, got %q", got)
	}
}

func TestResolvedVersionUsesBuildInfoRevision(t *testing.T) {
	orig := readBuildInfo
	t.Cleanup(func() {
		readBuildInfo = orig
	})
	readBuildInfo = func() (*debug.BuildInfo, bool) {
		return &debug.BuildInfo{
			Main: debug.Module{Version: goDevelMainVersion},
			Settings: []debug.BuildSetting{
				{Key: vcsRevisionKey, Value: "0123456789abcdef"},
				{Key: vcsModifiedKey, Value: "true"},
			},
		}, true
	}

	got := resolvedVersion("")
	if got != "0123456789ab-dirty" {
		t.Fatalf("expected vcs revision fallback, got %q", got)
	}
}

func TestResolvedVersionFallsBackToDev(t *testing.T) {
	orig := readBuildInfo
	t.Cleanup(func() {
		readBuildInfo = orig
	})
	readBuildInfo = func() (*debug.BuildInfo, bool) {
		return nil, false
	}

	got := resolvedVersion("")
	if got != devVersion {
		t.Fatalf("expected dev fallback, got %q", got)
	}
}
