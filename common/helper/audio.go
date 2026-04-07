package helper

import (
	"context"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
)

var (
	ffprobePath string
	ffprobeOnce sync.Once
	ffprobeErr  error
)

// ErrFFProbeUnavailable is returned when ffprobe/avprobe cannot be executed.
var ErrFFProbeUnavailable = errors.New("ffprobe unavailable")

// IsFFProbeUnavailable reports whether the given error indicates ffprobe is
// unavailable on the host.
func IsFFProbeUnavailable(err error) bool {
	return errors.Is(err, ErrFFProbeUnavailable)
}

func lookupFFProbe() (string, error) {
	ffprobeOnce.Do(func() {
		path, err := exec.LookPath("ffprobe")
		if err != nil {
			if alt, altErr := exec.LookPath("avprobe"); altErr == nil {
				ffprobePath = alt
				ffprobeErr = nil
				return
			}
			ffprobeErr = errors.Wrapf(ErrFFProbeUnavailable, "ffprobe not found in PATH: %v", err)
			return
		}
		ffprobePath = path
	})
	return ffprobePath, ffprobeErr
}

// SaveTmpFile saves data to a temporary file. The filename would be apppended with a random string.
func SaveTmpFile(filename string, data io.Reader) (string, error) {
	if data == nil {
		return "", errors.New("data is nil")
	}

	f, err := os.CreateTemp("", "*-"+filename)
	if err != nil {
		return "", errors.Wrapf(err, "failed to create temporary file %s", filename)
	}
	defer f.Close()

	_, err = io.Copy(f, data)
	if err != nil {
		return "", errors.Wrapf(err, "failed to copy data to temporary file %s", filename)
	}

	return f.Name(), nil
}

// GetAudioTokens returns the number of tokens in an audio file.
func GetAudioTokens(ctx context.Context, audio io.Reader, tokensPerSecond float64) (float64, error) {
	filename, err := SaveTmpFile("audio", audio)
	if err != nil {
		return 0, errors.Wrap(err, "failed to save audio to temporary file")
	}
	defer os.Remove(filename)

	duration, err := GetAudioDuration(ctx, filename)
	if err != nil {
		return 0, errors.Wrap(err, "failed to get audio tokens")
	}

	return duration * tokensPerSecond, nil
}

// GetAudioDuration returns the duration of an audio file in seconds.
func GetAudioDuration(ctx context.Context, filename string) (float64, error) {
	path, err := lookupFFProbe()
	if err != nil {
		gmw.GetLogger(ctx).Debug("ffprobe unavailable", zap.Error(err))
		return 0, errors.Wrap(err, "failed to get audio duration")
	}
	// ffprobe -v error -show_entries format=duration -of default=noprint_wrappers=1:nokey=1 {{input}}
	c := exec.CommandContext(ctx, path, "-v", "error", "-show_entries", "format=duration", "-of", "default=noprint_wrappers=1:nokey=1", filename)
	output, err := c.CombinedOutput()
	if err != nil {
		gmw.GetLogger(ctx).Debug("ffprobe execution failed",
			zap.String("ffprobe_path", path),
			zap.String("filename", filename),
			zap.String("stderr", strings.TrimSpace(string(output))),
			zap.Error(err))
		wrapped := wrapFFProbeError(err, output)
		return 0, errors.Wrap(wrapped, "failed to get audio duration")
	}

	trimmed := strings.TrimSpace(string(output))
	duration, err := strconv.ParseFloat(trimmed, 64)
	if err != nil {
		return 0, errors.Wrapf(err, "failed to parse ffprobe duration %q", trimmed)
	}

	// Actually gpt-4-audio calculates tokens with 0.1s precision,
	// while whisper calculates tokens with 1s precision
	return duration, nil
}

func wrapFFProbeError(err error, output []byte) error {
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		stderr := strings.TrimSpace(string(output))
		if exitErr.ExitCode() == 127 {
			if stderr == "" {
				stderr = "exit status 127"
			}
			return errors.Wrapf(ErrFFProbeUnavailable, "ffprobe exited with 127: %s", stderr)
		}

		if stderr != "" {
			return errors.Wrapf(err, "ffprobe stderr: %s", stderr)
		}
	}

	return errors.Wrap(err, "ffprobe error")
}
