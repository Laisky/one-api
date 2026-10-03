package groq

import (
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/relaymode"
)

// TestGroqDispatchParameterCompatibility covers all text protocol entry points
// after conversion and confirms that the body presented to dispatch is clean.
func TestGroqDispatchParameterCompatibility(t *testing.T) {
	t.Parallel()
	for _, mode := range []int{relaymode.ChatCompletions, relaymode.ClaudeMessages, relaymode.ResponseAPI} {
		body, err := prepareGroqRequestBody(&meta.Meta{Mode: mode}, strings.NewReader(`{"messages":[{"role":"user","name":"user","content":"hello"}],"logprobs":false,"top_logprobs":0,"temperature":0}`))
		require.NoError(t, err)
		wire, err := io.ReadAll(body)
		require.NoError(t, err)
		var root map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(wire, &root))
		require.NotContains(t, root, "logprobs")
		require.NotContains(t, root, "top_logprobs")
		require.Equal(t, `0`, string(root["temperature"]))
		require.JSONEq(t, `[{"role":"user","content":"hello"}]`, string(root["messages"]))
	}
}

// groqFailingBody exercises read failures and detects accidental buffering of
// non-text request modes before the normal multipart transport can process them.
type groqFailingBody struct{}

// Read returns a deterministic read failure without producing body bytes.
func (*groqFailingBody) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }

// TestGroqDispatchParameterCompatibilityBoundaries preserves opaque non-chat
// bodies and reports text-body read failures without dispatching a request.
func TestGroqDispatchParameterCompatibilityBoundaries(t *testing.T) {
	t.Parallel()
	original := &groqFailingBody{}
	body, err := prepareGroqRequestBody(nil, original)
	require.NoError(t, err)
	require.Same(t, original, body)
	body, err = prepareGroqRequestBody(&meta.Meta{Mode: relaymode.ImagesGenerations}, original)
	require.NoError(t, err)
	require.Same(t, original, body)
	body, err = prepareGroqRequestBody(&meta.Meta{Mode: relaymode.ChatCompletions}, nil)
	require.NoError(t, err)
	require.Nil(t, body)
	body, err = prepareGroqRequestBody(&meta.Meta{Mode: relaymode.ChatCompletions}, original)
	require.ErrorIs(t, err, io.ErrUnexpectedEOF)
	require.Nil(t, body)
}
