package anthropic

import (
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestClaude52StandardBillingBoundary rejects unpriced speed modes before upstream dispatch.
func TestClaude52StandardBillingBoundary(t *testing.T) {
	for _, name := range []string{"claude-fable-5-2", "claude-mythos-5-2"} {
		for _, speed := range []string{`"fast"`, `"future"`, `true`, `null`} {
			t.Run(name+"/"+speed, func(t *testing.T) {
				_, err := PrepareRequestBody(nil, name, strings.NewReader(`{"model":"`+name+`","speed":`+speed+`,"messages":[]}`))
				require.Error(t, err)
			})
		}
	}
}

// TestClaude52OpaqueReplay preserves native blocks, numeric precision, and legacy request behavior.
func TestClaude52OpaqueReplay(t *testing.T) {
	for _, name := range []string{"claude-fable-5-2", "claude-mythos-5-2", "claude-sonnet-4", "custom-model"} {
		for _, speed := range []string{"", `,"speed":"standard"`} {
			raw := `{"model":"` + name + `","messages":[{"role":"assistant","content":[{"type":"thinking","thinking":"opaque <>&","signature":"signed"},{"type":"redacted_thinking","data":"opaque"}]}],"future":{"id":9007199254740993}` + speed + `}`
			body, err := PrepareRequestBody(nil, name, strings.NewReader(raw))
			require.NoError(t, err)
			got, err := io.ReadAll(body)
			require.NoError(t, err)
			require.Equal(t, raw, string(got))
		}
	}
	original := strings.NewReader(`{"speed":"fast"}`)
	got, err := PrepareRequestBody(nil, "custom-model", original)
	require.NoError(t, err)
	require.Same(t, original, got)
}
