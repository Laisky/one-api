package common

import (
	"runtime/debug"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFinalizeVersionMetadataWithCommitAndTime(t *testing.T) {
	t.Parallel()
	info := &debug.BuildInfo{
		Main: debug.Module{Version: "(devel)"},
		Settings: []debug.BuildSetting{
			{Key: "vcs.revision", Value: "1234567890abcdef"},
			{Key: "vcs.time", Value: "2025-11-05T12:34:56Z"},
		},
	}

	versionDisplay, commit, buildTime := finalizeVersionMetadata("0.0.0", "", "", info)

	require.Equal(t, "2025-11-05T12:34:56Z (1234567)", versionDisplay)
	require.Equal(t, "1234567890abcdef", commit)
	require.Equal(t, "2025-11-05T12:34:56Z", buildTime)
}

func TestFinalizeVersionMetadataHonorsReleaseVersion(t *testing.T) {
	t.Parallel()
	info := &debug.BuildInfo{
		Main: debug.Module{Version: "v1.2.3"},
		Settings: []debug.BuildSetting{
			{Key: "vcs.revision", Value: "abcdef1"},
			{Key: "vcs.time", Value: "2025-11-05T12:34:56.789Z"},
		},
	}

	versionDisplay, commit, buildTime := finalizeVersionMetadata("0.0.0", "", "", info)

	require.Equal(t, "v1.2.3 2025-11-05T12:34:56Z (abcdef1)", versionDisplay)
	require.Equal(t, "abcdef1", commit)
	require.Equal(t, "2025-11-05T12:34:56Z", buildTime)
}

func TestFinalizeVersionMetadataPrefersManualOverrides(t *testing.T) {
	t.Parallel()
	info := &debug.BuildInfo{
		Main: debug.Module{Version: "v1.2.3"},
		Settings: []debug.BuildSetting{
			{Key: "vcs.revision", Value: "ignored"},
			{Key: "vcs.time", Value: "2025-11-05T12:34:56Z"},
		},
	}

	versionDisplay, commit, buildTime := finalizeVersionMetadata("release", "deadbeefcafebabe", "2025-11-04T00:00:00Z", info)

	require.Equal(t, "release 2025-11-04T00:00:00Z (deadbee)", versionDisplay)
	require.Equal(t, "deadbeefcafebabe", commit)
	require.Equal(t, "2025-11-04T00:00:00Z", buildTime)
}
