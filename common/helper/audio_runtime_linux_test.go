package helper

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// mediaRuntimeCase describes one baseline-observed synthetic probe fixture.
type mediaRuntimeCase struct {
	Name     string `json:"name"`
	Path     string `json:"path"`
	Success  bool   `json:"success"`
	Duration string `json:"duration"`
}

// TestMediaRuntimeProbeCorpus checks the actual helper on baseline observations.
func TestMediaRuntimeProbeCorpus(t *testing.T) {
	if os.Getenv("ONEAPI_MEDIA_RUNTIME") != "1" {
		t.Skip("set ONEAPI_MEDIA_RUNTIME=1 for explicit image qualification")
	}
	root := os.Getenv("ONEAPI_MEDIA_CORPUS_DIR")
	require.NotEmpty(t, root)
	data, err := os.ReadFile(filepath.Join(root, "manifest.json"))
	require.NoError(t, err)
	var corpus struct {
		Cases []mediaRuntimeCase `json:"cases"`
	}
	require.NoError(t, json.Unmarshal(data, &corpus))
	require.GreaterOrEqual(t, len(corpus.Cases), 20)
	for _, fixture := range corpus.Cases {
		t.Run(fixture.Name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			duration, err := GetAudioDuration(ctx, filepath.Join(root, fixture.Path))
			if !fixture.Success {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			expected, err := strconv.ParseFloat(fixture.Duration, 64)
			require.NoError(t, err)
			require.InDelta(t, expected, duration, 0.000001)
			content, err := os.ReadFile(filepath.Join(root, fixture.Path))
			require.NoError(t, err)
			// bytes.Buffer has no Seek method: materialization must make it seekable.
			tokens, err := GetAudioTokens(ctx, bytes.NewBuffer(content), 50)
			require.NoError(t, err)
			require.InDelta(t, expected*50, tokens, 0.00005)
		})
	}
}

// TestMediaRuntimeProbeCancellation verifies cancellation reaps a blocked real probe.
func TestMediaRuntimeProbeCancellation(t *testing.T) {
	if os.Getenv("ONEAPI_MEDIA_RUNTIME") != "1" {
		t.Skip("set ONEAPI_MEDIA_RUNTIME=1 for explicit image qualification")
	}
	fifo := filepath.Join(t.TempDir(), "synthetic-blocked-input")
	require.NoError(t, syscall.Mkfifo(fifo, 0o600))
	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := GetAudioDuration(ctx, fifo)
	require.Error(t, err)
	require.ErrorIs(t, ctx.Err(), context.DeadlineExceeded)
	require.Less(t, time.Since(start), 3*time.Second)
	children, err := filepath.Glob("/proc/self/task/*/children")
	require.NoError(t, err)
	for _, path := range children {
		data, err := os.ReadFile(path)
		if os.IsNotExist(err) {
			continue
		}
		require.NoError(t, err)
		require.Empty(t, strings.TrimSpace(string(data)), "probe child must be reaped")
	}
}

// TestMediaRuntimeWAVSamples checks the synthetic PCM fixture's independent duration.
func TestMediaRuntimeWAVSamples(t *testing.T) {
	if os.Getenv("ONEAPI_MEDIA_RUNTIME") != "1" {
		t.Skip("set ONEAPI_MEDIA_RUNTIME=1 for explicit image qualification")
	}
	data, err := os.ReadFile(filepath.Join(os.Getenv("ONEAPI_MEDIA_CORPUS_DIR"), "tone.wav"))
	require.NoError(t, err)
	require.Equal(t, uint32(24000), binary.LittleEndian.Uint32(data[24:28]))
	require.Equal(t, 2*24000*2, len(data)-44)
	duration, err := GetAudioDuration(context.Background(), filepath.Join(os.Getenv("ONEAPI_MEDIA_CORPUS_DIR"), "tone.wav"))
	require.NoError(t, err)
	require.Equal(t, float64(2), duration)
}
