package helper

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	gmw "github.com/Laisky/gin-middlewares/v7"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/Laisky/zap"
	"github.com/Laisky/zap/zapcore"
	"github.com/Laisky/zap/zaptest/observer"
	"github.com/stretchr/testify/require"
)

// TestAudioFailureReportedOnce checks returned probe errors and retained debug
// context in isolated processes so the once-only lookup cannot affect other tests.
func TestAudioFailureReportedOnce(t *testing.T) {
	mode := os.Getenv("ONEAPI_TEST_AUDIO_LOG_MODE")
	if mode == "" {
		for _, m := range []string{"missing", "failure"} {
			t.Run(m, func(t *testing.T) {
				cmd := exec.Command(os.Args[0], "-test.run=^TestAudioFailureReportedOnce$")
				cmd.Env = append(os.Environ(), "ONEAPI_TEST_AUDIO_LOG_MODE="+m)
				out, err := cmd.CombinedOutput()
				require.NoError(t, err, "%s", out)
			})
		}
		return
	}
	dir := t.TempDir()
	if mode == "failure" {
		require.NoError(t, os.WriteFile(filepath.Join(dir, "ffprobe"), []byte("#!/bin/sh\necho synthetic-stderr >&2\nexit 1\n"), 0700))
	}
	t.Setenv("PATH", dir)
	core, logs := observer.New(zapcore.DebugLevel)
	lg, err := glog.NewWithName("audio-test", glog.LevelDebug, zap.WrapCore(func(zapcore.Core) zapcore.Core { return core }))
	require.NoError(t, err)
	_, err = GetAudioDuration(gmw.SetLogger(context.Background(), lg), "synthetic-audio")
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to get audio duration")
	if mode == "missing" {
		require.ErrorIs(t, err, ErrFFProbeUnavailable)
		require.Empty(t, logs.FilterMessage("ffprobe unavailable").All())
	} else {
		require.Contains(t, err.Error(), "synthetic-stderr")
		entries := logs.FilterMessage("ffprobe execution failed").All()
		require.Len(t, entries, 1)
		fields := entries[0].ContextMap()
		require.Equal(t, "synthetic-audio", fields["filename"])
		require.Equal(t, "synthetic-stderr", fields["stderr"])
		require.NotContains(t, fields, "error")
	}
}
