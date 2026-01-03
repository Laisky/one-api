package main

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/stretchr/testify/require"
)

func TestParseAudioArgsDefault(t *testing.T) {
	t.Parallel()
	opts, err := parseAudioArgs(nil)
	require.NoError(t, err)
	require.Equal(t, defaultAudioSampleURL, opts.source)
	require.Equal(t, "default-sample", opts.displaySource)
}

func TestRunAudioProbeLocalFile(t *testing.T) {
	t.Parallel()
	tmp, err := os.CreateTemp("", "audio-probe-test-")
	require.NoError(t, err)
	data := []byte("fake audio data for probing")
	_, err = tmp.Write(data)
	require.NoError(t, err)
	require.NoError(t, tmp.Close())
	defer os.Remove(tmp.Name())

	logger, err := glog.NewConsoleWithName("audio-test", glog.LevelInfo)
	require.NoError(t, err)

	calledDuration := false
	calledTokens := false

	durationFn := func(ctx context.Context, filename string) (float64, error) {
		calledDuration = true
		require.Equal(t, tmp.Name(), filename)
		return 1.23, nil
	}

	tokensFn := func(ctx context.Context, reader io.Reader, rate float64) (float64, error) {
		calledTokens = true
		payload, err := io.ReadAll(reader)
		require.NoError(t, err)
		require.Equal(t, data, payload)
		require.Equal(t, defaultAudioTokensPerSecond, rate)
		return 61.5, nil
	}

	err = runAudioProbe(context.Background(), logger, []string{tmp.Name()}, durationFn, tokensFn)
	require.NoError(t, err)
	require.True(t, calledDuration)
	require.True(t, calledTokens)

	_, statErr := os.Stat(tmp.Name())
	require.NoError(t, statErr)
}

func TestMaterializeAudioSourceRemote(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("remote audio"))
	}))
	t.Cleanup(srv.Close)

	logger, err := glog.NewConsoleWithName("audio-remote", glog.LevelInfo)
	require.NoError(t, err)

	path, cleanup, err := materializeAudioSource(context.Background(), logger, srv.URL+"/audio.m4a")
	require.NoError(t, err)
	require.NotEmpty(t, path)

	contents, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, []byte("remote audio"), contents)

	cleanup()
	_, statErr := os.Stat(path)
	require.Error(t, statErr)
	require.True(t, os.IsNotExist(statErr))
}

func TestMaterializeAudioSourceLocal(t *testing.T) {
	t.Parallel()
	tmp, err := os.CreateTemp("", "audio-local-")
	require.NoError(t, err)
	_, err = tmp.Write([]byte("local audio"))
	require.NoError(t, err)
	require.NoError(t, tmp.Close())
	t.Cleanup(func() {
		_ = os.Remove(tmp.Name())
	})

	logger, err := glog.NewConsoleWithName("audio-local", glog.LevelInfo)
	require.NoError(t, err)

	path, cleanup, err := materializeAudioSource(context.Background(), logger, tmp.Name())
	require.NoError(t, err)
	require.Equal(t, tmp.Name(), path)

	cleanup()
	_, statErr := os.Stat(tmp.Name())
	require.NoError(t, statErr)
}
