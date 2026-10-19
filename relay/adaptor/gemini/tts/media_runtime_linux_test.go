package tts

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/stretchr/testify/require"
)

// mediaRuntimePCM produces non-silent mono PCM16LE with a known physical duration.
func mediaRuntimePCM(seconds int) []byte {
	pcm := make([]byte, seconds*24000*2)
	for index := 0; index < len(pcm)/2; index++ {
		sample := int16(10000 * math.Sin(2*math.Pi*440*float64(index)/24000))
		binary.LittleEndian.PutUint16(pcm[index*2:], uint16(sample))
	}
	return pcm
}

// mediaRuntimeCommand runs a real bounded utility and fails on unavailable codecs.
func mediaRuntimeCommand(t *testing.T, program string, args ...string) []byte {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, program, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	data, err := cmd.Output()
	require.NoError(t, err, "%s: %s", program, stderr.String())
	return data
}

// TestMediaRuntimeSpeechMatrix validates actual encoded audio for all output paths.
func TestMediaRuntimeSpeechMatrix(t *testing.T) {
	if os.Getenv("ONEAPI_MEDIA_RUNTIME") != "1" {
		t.Skip("set ONEAPI_MEDIA_RUNTIME=1 for explicit image qualification")
	}
	encoder, err := exec.LookPath("ffmpeg")
	require.NoError(t, err, "explicit runtime qualification cannot skip codecs")
	probe, err := exec.LookPath("ffprobe")
	require.NoError(t, err)
	pcm := mediaRuntimePCM(3)
	for _, format := range []string{"mp3", "opus", "aac", "flac", "wav", "pcm"} {
		for _, speed := range []float64{0.25, 1, 4} {
			t.Run(fmt.Sprintf("%s_speed_%g", format, speed), func(t *testing.T) {
				plan := &Plan{Encoder: encoder, Format: format, Speed: speed}
				encoded, err := plan.encode(context.Background(), pcm)
				require.NoError(t, err)
				require.NotEmpty(t, encoded)
				if speed == 1 && format == "pcm" {
					require.Equal(t, pcm, encoded)
				}
				path := filepath.Join(t.TempDir(), "speech."+format)
				require.NoError(t, os.WriteFile(path, encoded, 0o600))
				raw := []string{}
				if format == "pcm" {
					raw = []string{"-f", "s16le", "-ar", "24000", "-ac", "1"}
				} else {
					args := []string{"-v", "error", "-select_streams", "a:0", "-show_entries", "stream=codec_name,sample_rate,channels", "-of", "json", path}
					var info struct {
						Streams []struct {
							Codec      string `json:"codec_name"`
							SampleRate string `json:"sample_rate"`
							Channels   int    `json:"channels"`
						} `json:"streams"`
					}
					require.NoError(t, json.Unmarshal(mediaRuntimeCommand(t, probe, args...), &info))
					require.Len(t, info.Streams, 1)
					require.Equal(t, 1, info.Streams[0].Channels)
					rate := "24000"
					if format == "opus" {
						rate = "48000"
					}
					require.Equal(t, rate, info.Streams[0].SampleRate)
					codec := map[string]string{"mp3": "mp3", "opus": "opus", "aac": "aac", "flac": "flac", "wav": "pcm_s16le"}[format]
					require.Equal(t, codec, info.Streams[0].Codec)
				}
				args := append([]string{"-hide_banner", "-v", "error", "-nostdin", "-threads", "1"}, raw...)
				args = append(args, "-i", path, "-map", "0:a:0", "-ac", "1", "-ar", "24000", "-threads", "1", "-f", "s16le", "pipe:1")
				decoded := mediaRuntimeCommand(t, encoder, args...)
				require.Zero(t, len(decoded)%2)
				seconds := float64(len(decoded)) / 48000
				expected := 3 / speed
				require.InDelta(t, expected, seconds, math.Max(0.15, expected*0.03))
				var energy float64
				for index := 0; index < len(decoded); index += 2 {
					value := float64(int16(binary.LittleEndian.Uint16(decoded[index:]))) / 32768
					energy += value * value
				}
				require.Greater(t, energy/float64(len(decoded)/2), 0.001, "decoded tone must remain audible")
				if output := os.Getenv("ONEAPI_MEDIA_OUTPUT_DIR"); output != "" {
					name := fmt.Sprintf("%s_speed_%g.%s", format, speed, format)
					require.NoError(t, os.WriteFile(filepath.Join(output, name), encoded, 0o644))
					report, err := json.Marshal(map[string]any{
						"format": format, "speed": speed, "decoded_bytes": len(decoded),
						"decoded_seconds":        seconds,
						"decoded_sha256":         fmt.Sprintf("%x", sha256.Sum256(decoded)),
						"normalized_sample_rate": 24000, "channels": 1,
					})
					require.NoError(t, err)
					require.NoError(t, os.WriteFile(filepath.Join(output, name+".json"), report, 0o644))
				}
				t.Logf("format=%s speed=%g decoded_seconds=%.6f normalized_rate=24000", format, speed, seconds)
			})
		}
	}
}

// mediaRuntimeChildren returns the process children of every current Go thread.
func mediaRuntimeChildren() ([]int, error) {
	paths, err := filepath.Glob("/proc/self/task/*/children")
	if err != nil {
		return nil, errors.Wrap(err, "read synthetic encoder process state")
	}
	var children []int
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, errors.Wrap(err, "read synthetic encoder process state")
		}
		for _, value := range strings.Fields(string(data)) {
			pid, err := strconv.Atoi(value)
			if err != nil {
				return nil, errors.Wrap(err, "read synthetic encoder process state")
			}
			children = append(children, pid)
		}
	}
	return children, nil
}

// TestMediaRuntimeEncoderCancellation kills an executing real encoder and checks reaping.
func TestMediaRuntimeEncoderCancellation(t *testing.T) {
	if os.Getenv("ONEAPI_MEDIA_RUNTIME") != "1" {
		t.Skip("set ONEAPI_MEDIA_RUNTIME=1 for explicit image qualification")
	}
	encoder, err := exec.LookPath("ffmpeg")
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	pcm := bytes.Repeat(mediaRuntimePCM(1), 120)
	finished := make(chan error, 1)
	go func() {
		_, err := (&Plan{Encoder: encoder, Format: "opus", Speed: 0.25}).encode(ctx, pcm)
		finished <- err
	}()
	var pid int
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		children, err := mediaRuntimeChildren()
		require.NoError(t, err)
		for _, child := range children {
			maps, err := os.ReadFile(fmt.Sprintf("/proc/%d/maps", child))
			if err != nil {
				continue
			}
			for _, line := range strings.Split(string(maps), "\n") {
				if strings.HasSuffix(line, "/ffmpeg") {
					pid = child
				}
			}
		}
		if pid != 0 {
			break
		}
		select {
		case err := <-finished:
			t.Fatalf("encoder finished before cancellation could be exercised: %v", err)
		default:
		}
		time.Sleep(5 * time.Millisecond)
	}
	cancel()
	require.NotZero(t, pid, "must observe the actual executable, not only its wrapper")
	select {
	case err := <-finished:
		require.Error(t, err)
	case <-time.After(3 * time.Second):
		t.Fatal("cancelled encoder did not return promptly")
	}
	_, err = os.Stat(fmt.Sprintf("/proc/%d", pid))
	require.True(t, os.IsNotExist(err), "cancelled encoder must be reaped")
	children, err := mediaRuntimeChildren()
	require.NoError(t, err)
	require.Empty(t, children)
}
