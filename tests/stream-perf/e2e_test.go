package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// runE2ECommand runs a bounded fixture command from directory and returns its combined output to the test log.
func runE2ECommand(t *testing.T, ctx context.Context, directory string, environment []string, name string, args ...string) {
	t.Helper()
	command := exec.CommandContext(ctx, name, args...)
	command.Dir = directory
	command.Env = environment
	command.WaitDelay = 5 * time.Second
	output, err := command.CombinedOutput()
	require.NoError(t, err, "%s failed:\n%s", name, output)
	t.Logf("%s output:\n%s", name, output)
}

// streamingE2EUnavailable returns the first unmet host prerequisite, or an empty string when the test can run.
// Required runs may prepare a fresh tokenizer cache; ordinary local tests never download tokenizer assets.
func streamingE2EUnavailable(short bool, goos string, pythonAvailable bool, cache string, required bool) string {
	switch {
	case short:
		return "streaming E2E builds the gateway; omit -short to run it"
	case goos != "linux":
		return "streaming E2E requires Linux /proc resource accounting"
	case !pythonAvailable:
		return "streaming E2E requires python3 on PATH"
	case cache == "" && !required:
		return "prepare tokenizer assets with cache_tokens.py and set TIKTOKEN_CACHE_DIR to run streaming E2E offline"
	default:
		return ""
	}
}

// TestStreamingE2EPrerequisites checks local skips and required-run failures without invoking external commands.
func TestStreamingE2EPrerequisites(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, goos, cache string
		short, python, required, unavailable bool
	}{
		{name: "prepared local", goos: "linux", cache: "/cache", python: true},
		{name: "offline unprepared local", goos: "linux", python: true, unavailable: true},
		{name: "required preparation", goos: "linux", python: true, required: true},
		{name: "short local", goos: "linux", cache: "/cache", short: true, python: true, unavailable: true},
		{name: "short required", goos: "linux", short: true, python: true, required: true, unavailable: true},
		{name: "macOS local", goos: "darwin", cache: "/cache", python: true, unavailable: true},
		{name: "macOS required", goos: "darwin", python: true, required: true, unavailable: true},
		{name: "missing python local", goos: "linux", cache: "/cache", unavailable: true},
		{name: "missing python required", goos: "linux", required: true, unavailable: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reason := streamingE2EUnavailable(tc.short, tc.goos, tc.python, tc.cache, tc.required)
			require.Equal(t, tc.unavailable, reason != "")
		})
	}
}

// TestStreamingGatewayE2E builds the real gateway and runs HTTP correctness and bounded concurrent streaming smoke loads.
// GitHub Actions and ONEAPI_REQUIRE_STREAM_E2E=1 fail rather than skip when prerequisites are missing.
func TestStreamingGatewayE2E(t *testing.T) {
	required := os.Getenv("GITHUB_ACTIONS") == "true" || os.Getenv("ONEAPI_REQUIRE_STREAM_E2E") == "1"
	cache := os.Getenv("TIKTOKEN_CACHE_DIR")
	_, pythonErr := exec.LookPath("python3")
	if reason := streamingE2EUnavailable(testing.Short(), runtime.GOOS, pythonErr == nil, cache, required); reason != "" {
		if required {
			t.Fatalf("required streaming E2E cannot run: %s", reason)
		}
		t.Skip(reason)
	}
	directory, err := os.Getwd()
	require.NoError(t, err)
	root := filepath.Clean(filepath.Join(directory, "..", ".."))
	temporary := t.TempDir()
	driver := filepath.Join(temporary, "stream-perf")
	gateway := filepath.Join(temporary, "one-api")
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()
	environment := os.Environ()
	// Only required runs with no supplied cache may download pinned public assets during setup.
	// A supplied missing or corrupt cache is an error, not permission to silently use the network.
	if cache == "" {
		cache = filepath.Join(temporary, "token-cache")
		runE2ECommand(t, ctx, directory, environment, "python3", "cache_tokens.py", "--cache", cache)
	}
	runE2ECommand(t, ctx, directory, environment, "python3", "cache_tokens.py", "--cache", cache, "--check-only")
	runE2ECommand(t, ctx, root, environment, "go", "build", "-trimpath", "-o", driver, "./tests/stream-perf")
	runE2ECommand(t, ctx, root, environment, "go", "build", "-trimpath", "-o", gateway, ".")
	runE2ECommand(t, ctx, directory, environment, "python3", "-m", "unittest", "discover", "-v", "-s", ".", "-p", "test_*.py")
	runE2ECommand(t, ctx, directory, environment, "python3", "run.py", "--binary", gateway, "--driver", driver,
		"--token-cache", cache, "--output", filepath.Join(temporary, "results"), "--concurrency", "1,8",
		"--requests", "16", "--paced-requests", "16", "--chunks", "32", "--repeats", "1")
}
