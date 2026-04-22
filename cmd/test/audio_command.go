package main

import (
	"context"
	"flag"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	githubErrors "github.com/Laisky/errors/v2"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/Laisky/zap"

	"github.com/songquanpeng/one-api/common/helper"
)

const (
	defaultAudioSampleURL       = "https://s3.laisky.com/uploads/2025/01/audio-sample.m4a"
	defaultAudioTokensPerSecond = 50.0
)

type audioDurationFunc func(ctx context.Context, filename string) (float64, error)
type audioTokensFunc func(ctx context.Context, reader io.Reader, tokensPerSecond float64) (float64, error)

// audio verifies that ffprobe/ffmpeg are available by measuring an audio sample. The args slice optionally supplies a local file path or HTTP(S) URL to probe. It returns an error if either probe fails.
func audio(ctx context.Context, logger glog.Logger, args []string) error {
	return runAudioProbe(ctx, logger, args, helper.GetAudioDuration, helper.GetAudioTokens)
}

// runAudioProbe orchestrates the duration and token probes. Args may override the default audio source. The durationFn and tokensFn parameters allow calling code and tests to inject alternative implementations. It returns an error when the probe fails.
func runAudioProbe(ctx context.Context, logger glog.Logger, args []string, durationFn audioDurationFunc, tokensFn audioTokensFunc) error {
	opts, err := parseAudioArgs(args)
	if err != nil {
		return githubErrors.Wrap(err, "parse audio arguments")
	}

	audioPath, cleanup, err := materializeAudioSource(ctx, logger, opts.source)
	if err != nil {
		return githubErrors.Wrap(err, "prepare audio source")
	}
	defer cleanup()

	duration, err := durationFn(ctx, audioPath)
	if err != nil {
		if helper.IsFFProbeUnavailable(err) {
			return githubErrors.Wrap(err, "ffprobe unavailable; install ffmpeg/ffprobe and ensure it is in PATH")
		}
		return githubErrors.Wrap(err, "measure audio duration")
	}

	logger.Info("audio duration measured", zap.String("source", opts.displaySource), zap.String("local_path", audioPath), zap.Float64("seconds", duration))

	file, err := os.Open(audioPath)
	if err != nil {
		return githubErrors.Wrapf(err, "open audio file %s", audioPath)
	}
	defer file.Close()

	tokens, err := tokensFn(ctx, file, defaultAudioTokensPerSecond)
	if err != nil {
		if helper.IsFFProbeUnavailable(err) {
			return githubErrors.Wrap(err, "ffprobe unavailable; install ffmpeg/ffprobe and ensure it is in PATH")
		}
		return githubErrors.Wrap(err, "estimate audio tokens")
	}

	logger.Info("audio tokens estimated", zap.String("source", opts.displaySource), zap.Float64("tokens", tokens), zap.Float64("tokens_per_second", defaultAudioTokensPerSecond))

	logger.Info("audio probe succeeded", zap.String("source", opts.displaySource), zap.Float64("duration_seconds", duration), zap.Float64("token_estimate", tokens))

	return nil
}

// audioOptions describes the parsed command-line settings for the audio probe.
type audioOptions struct {
	source        string
	displaySource string
}

// parseAudioArgs resolves the requested audio source from CLI flags and positional arguments.
func parseAudioArgs(args []string) (audioOptions, error) {
	fs := flag.NewFlagSet("audio", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	var (
		sourceFlag string
	)

	fs.StringVar(&sourceFlag, "source", "", "audio file path or HTTP(S) URL to probe")
	fs.StringVar(&sourceFlag, "input", "", "alias for --source")

	if err := fs.Parse(args); err != nil {
		return audioOptions{}, githubErrors.Wrap(err, "parse audio flags")
	}

	source := strings.TrimSpace(sourceFlag)
	if source == "" && fs.NArg() > 0 {
		source = strings.TrimSpace(fs.Arg(0))
	}
	if source == "" {
		source = defaultAudioSampleURL
	}

	return audioOptions{source: source, displaySource: sourceDisplayLabel(source)}, nil
}

// sourceDisplayLabel normalizes source strings for logging while avoiding leaking temp paths for the default sample.
func sourceDisplayLabel(source string) string {
	trimmed := strings.TrimSpace(source)
	if trimmed == "" {
		return defaultAudioSampleURL
	}
	if trimmed == defaultAudioSampleURL {
		return "default-sample"
	}
	return trimmed
}

// materializeAudioSource ensures the audio source is accessible locally and returns the path and a cleanup function for temporary files. The cleanup should always be invoked to avoid leaking temporary artifacts.
func materializeAudioSource(ctx context.Context, logger glog.Logger, source string) (string, func(), error) {
	trimmed := strings.TrimSpace(source)
	if trimmed == "" {
		return "", func() {}, githubErrors.New("audio source is empty")
	}

	u, err := url.Parse(trimmed)
	if err == nil && u.Scheme != "" && u.Host != "" {
		if !strings.EqualFold(u.Scheme, "http") && !strings.EqualFold(u.Scheme, "https") {
			return "", func() {}, githubErrors.Errorf("unsupported URI scheme %q", u.Scheme)
		}
		path, cleanup, dlErr := downloadAudio(ctx, logger, trimmed)
		if dlErr != nil {
			return "", func() {}, dlErr
		}
		return path, cleanup, nil
	}

	if _, statErr := os.Stat(trimmed); statErr != nil {
		return "", func() {}, githubErrors.Wrapf(statErr, "failed to stat audio source %s", trimmed)
	}

	return trimmed, func() {}, nil
}

// downloadAudio fetches the audio resource to a temporary file. The returned cleanup removes the file once the probe finishes.
func downloadAudio(ctx context.Context, logger glog.Logger, source string) (string, func(), error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, source, nil)
	if err != nil {
		return "", func() {}, githubErrors.Wrap(err, "construct audio request")
	}

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", func() {}, githubErrors.Wrap(err, "download audio sample")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", func() {}, githubErrors.Errorf("unexpected status %s from %s", resp.Status, source)
	}

	tmp, err := os.CreateTemp("", "oneapi-audio-")
	if err != nil {
		return "", func() {}, githubErrors.Wrap(err, "create temporary audio file")
	}

	success := false
	defer func() {
		if success {
			return
		}
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
	}()

	if _, err := io.Copy(tmp, resp.Body); err != nil {
		return "", func() {}, githubErrors.Wrap(err, "copy audio payload to disk")
	}

	if err := tmp.Close(); err != nil {
		return "", func() {}, githubErrors.Wrap(err, "flush temporary audio file")
	}

	success = true
	path := tmp.Name()
	cleanup := func() {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			logger.Warn("failed to remove temporary audio file", zap.String("path", path), zap.Error(err))
		}
	}

	return path, cleanup, nil
}
