package model

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestVideoRequestRequestedDurationSeconds(t *testing.T) {
	t.Parallel()
	seconds := 8.5
	duration := 6.0
	req := &VideoRequest{
		Seconds:         &seconds,
		Duration:        &duration,
		DurationSeconds: nil,
	}
	require.Equal(t, seconds, req.RequestedDurationSeconds(), "expected seconds")

	req.DurationSeconds = &duration
	require.Equal(t, duration, req.RequestedDurationSeconds(), "expected duration_seconds")

	req.DurationSeconds = nil
	req.Seconds = nil
	require.Equal(t, duration, req.RequestedDurationSeconds(), "expected duration fallback")
}

func TestVideoRequestRequestedResolution(t *testing.T) {
	t.Parallel()
	req := &VideoRequest{Size: "1280x720"}
	require.Equal(t, "1280x720", req.RequestedResolution(), "expected size resolution")

	req.Size = ""
	req.Resolution = "720x1280"
	require.Equal(t, "720x1280", req.RequestedResolution(), "expected fallback resolution")

	req.Resolution = ""
	require.Equal(t, "", req.RequestedResolution(), "expected empty resolution")
}
