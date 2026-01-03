package helper

import (
	"context"
	"io"
	"net/http"
	"os"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGetAudioDuration(t *testing.T) {
	t.Parallel()
	// skip if there is no ffprobe installed
	if _, err := exec.LookPath("ffprobe"); err != nil {
		if _, altErr := exec.LookPath("avprobe"); altErr != nil {
			t.Skip("ffprobe not installed, skipping test")
		}
	}

	t.Run("should return correct duration for a valid audio file", func(t *testing.T) {
		t.Parallel()
		tmpFile, err := os.CreateTemp("", "test_audio*.mp3")
		require.NoError(t, err)
		defer os.Remove(tmpFile.Name())

		// download test audio file
		resp, err := http.Get("https://s3.laisky.com/uploads/2025/01/audio-sample.m4a")
		if err != nil {
			t.Skipf("skipping audio duration test due to download failure: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Skipf("skipping audio duration test due to unexpected status: %s", resp.Status)
		}

		_, err = io.Copy(tmpFile, resp.Body)
		require.NoError(t, err)
		require.NoError(t, tmpFile.Close())

		duration, err := GetAudioDuration(context.Background(), tmpFile.Name())
		require.NoError(t, err)
		require.Equal(t, duration, 3.904)
	})

	t.Run("should return an error for a non-existent file", func(t *testing.T) {
		t.Parallel()
		_, err := GetAudioDuration(context.Background(), "non_existent_file.mp3")
		require.Error(t, err)
	})
}

func TestGetAudioTokens(t *testing.T) {
	t.Parallel()
	if _, err := exec.LookPath("ffprobe"); err != nil {
		if _, altErr := exec.LookPath("avprobe"); altErr != nil {
			t.Skip("ffprobe not installed, skipping test")
		}
	}

	t.Run("should return correct tokens for a valid audio file", func(t *testing.T) {
		t.Parallel()
		// download test audio file
		resp, err := http.Get("https://s3.laisky.com/uploads/2025/01/audio-sample.m4a")
		if err != nil {
			t.Skipf("skipping audio tokens test due to download failure: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Skipf("skipping audio tokens test due to unexpected status: %s", resp.Status)
		}

		tokens, err := GetAudioTokens(context.Background(), resp.Body, 50)
		require.NoError(t, err)
		require.Equal(t, tokens, 195.2)
	})

	t.Run("should return an error for a non-existent file", func(t *testing.T) {
		t.Parallel()
		_, err := GetAudioTokens(context.Background(), nil, 1)
		require.Error(t, err)
	})
}
