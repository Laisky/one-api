package webperf

import (
	"context"
	"os/exec"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestWebPerformanceContracts runs deterministic Web API fixture, oracle, evidence and transport contracts.
// Full multi-minute performance measurements remain an explicit CLI operation, not a noisy CI threshold.
func TestWebPerformanceContracts(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Web performance telemetry contracts require Linux /proc")
	}
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 is required for the optional local Web performance harness")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, python, "-B", "-m", "unittest", "discover", "-v", "-s", ".", "-p", "test_*.py")
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "Web performance contracts failed:\n%s", output)
	t.Logf("%s", output)
}
