package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
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
	output, err := command.CombinedOutput()
	require.NoError(t, err, "%s failed:\n%s", name, output)
	t.Logf("%s output:\n%s", name, output)
}

// TestStreamingGatewayE2E builds the real gateway and runs HTTP correctness and bounded concurrent streaming smoke loads.
// It is discovered by the existing go test ./... CI entrypoint; performance acceptance never relies on a noisy fixed RPS gate.
func TestStreamingGatewayE2E(t *testing.T) {
	directory, err := os.Getwd()
	require.NoError(t, err)
	root := filepath.Clean(filepath.Join(directory, "..", ".."))
	temporary := t.TempDir()
	driver := filepath.Join(temporary, "stream-perf")
	gateway := filepath.Join(temporary, "one-api")
	cache := filepath.Join(temporary, "token-cache")
	if existing := os.Getenv("TIKTOKEN_CACHE_DIR"); existing != "" {
		cache = existing
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()
	environment := os.Environ()
	runE2ECommand(t, ctx, root, environment, "go", "build", "-trimpath", "-o", driver, "./tests/stream-perf")
	runE2ECommand(t, ctx, root, environment, "go", "build", "-trimpath", "-o", gateway, ".")
	runE2ECommand(t, ctx, directory, environment, "python3", "-m", "unittest", "discover", "-v", "-s", ".", "-p", "test_*.py")
	runE2ECommand(t, ctx, directory, environment, "python3", "cache_tokens.py", "--cache", cache)
	runE2ECommand(t, ctx, directory, environment, "python3", "run.py", "--binary", gateway, "--driver", driver,
		"--token-cache", cache, "--output", filepath.Join(temporary, "results"), "--concurrency", "1,8",
		"--requests", "16", "--paced-requests", "16", "--chunks", "32", "--repeats", "1")
}
