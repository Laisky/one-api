package mcp

import (
	"encoding/json"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestModelVisibleJSON verifies that only intentional tool payloads cross the
// model boundary and that transport serialization remains unchanged.
func TestModelVisibleJSON(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{"modern", `{"content":[{"type":"text","text":"answer"}],"structuredContent":{"n":9007199254740993},"isError":true,"_meta":{"secret":"client-only"},"debug":"private"}`, `{"content":[{"type":"text","text":"answer"}],"structuredContent":{"n":9007199254740993},"isError":true}`},
		{"legacy", `{"content":[],"structured_content":{"ok":true},"is_error":true,"request_state":"client-only","input_requests":{"form":{}}}`, `{"content":[],"structuredContent":{"ok":true},"isError":true}`},
		{"explicit-null", `{"structuredContent":null,"structured_content":{"ignored":true},"isError":false,"is_error":true}`, `{"content":null,"structuredContent":null}`},
		{"interaction-only", `{"resultType":"input_required","requestState":"client-only","inputRequests":{"x":{}}}`, `{"content":null}`},
		{"application-payload", `{"content":[{"type":"image","mimeType":"image/png","data":"fixture"}],"structuredContent":{"_meta":"intentional application data"}}`, `{"content":[{"type":"image","mimeType":"image/png","data":"fixture"}],"structuredContent":{"_meta":"intentional application data"}}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var result CallToolResult
			require.NoError(t, json.Unmarshal([]byte(tc.raw), &result))
			before, err := json.Marshal(result)
			require.NoError(t, err)
			rawBefore := append([]byte(nil), result.Raw...)
			visible, err := result.ModelVisibleJSON()
			require.NoError(t, err)
			require.JSONEq(t, tc.want, string(visible))
			if tc.name == "modern" {
				require.Contains(t, string(visible), "9007199254740993", "integers must not round through float64")
			}
			after, err := json.Marshal(result)
			require.NoError(t, err)
			require.Equal(t, before, after)
			require.Equal(t, rawBefore, []byte(result.Raw))
		})
	}
}

// TestModelVisibleJSONRawOnly verifies legacy Raw-only construction, malformed
// input rejection, and the precedence of deliberately updated typed content.
func TestModelVisibleJSONRawOnly(t *testing.T) {
	result := &CallToolResult{Raw: json.RawMessage(`{"content":"answer","structured_content":{"n":9007199254740993},"_meta":{"secret":"private"}}`)}
	visible, err := result.ModelVisibleJSON()
	require.NoError(t, err)
	require.NotContains(t, string(visible), "private")
	require.Contains(t, string(visible), "9007199254740993")
	require.Nil(t, result.Content, "projection cannot populate the original result")
	result.Content = "updated"
	visible, err = result.ModelVisibleJSON()
	require.NoError(t, err)
	require.JSONEq(t, `{"content":"updated"}`, string(visible))

	for _, raw := range []string{`{`, `[]`, `{"isError":"invalid"}`} {
		_, err := (&CallToolResult{Raw: json.RawMessage(raw)}).ModelVisibleJSON()
		require.Error(t, err)
	}
	_, err = (&CallToolResult{Content: make(chan int)}).ModelVisibleJSON()
	require.Error(t, err)
	visible, err = (&CallToolResult{Content: "valid", AdditionalFields: map[string]any{"private": make(chan int)}}).ModelVisibleJSON()
	require.NoError(t, err, "non-model extensions must not be serialized")
	require.JSONEq(t, `{"content":"valid"}`, string(visible))
	var empty *CallToolResult
	visible, err = empty.ModelVisibleJSON()
	require.NoError(t, err)
	require.JSONEq(t, `{"content":null}`, string(visible))
}

// TestModelVisibleJSONConcurrent verifies that concurrent projections only read
// a shared result and return independent encodings without mutating Raw.
func TestModelVisibleJSONConcurrent(t *testing.T) {
	var result CallToolResult
	require.NoError(t, json.Unmarshal([]byte(`{"content":"answer","structuredContent":{"n":1},"_meta":{"secret":"private"}}`), &result))
	before := append([]byte(nil), result.Raw...)
	var wg sync.WaitGroup
	outputs := make(chan []byte, 32)
	errors := make(chan error, 32)
	for range 32 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			encoded, err := result.ModelVisibleJSON()
			outputs <- encoded
			errors <- err
		}()
	}
	wg.Wait()
	close(outputs)
	close(errors)
	for err := range errors {
		require.NoError(t, err)
	}
	for encoded := range outputs {
		require.JSONEq(t, `{"content":"answer","structuredContent":{"n":1}}`, string(encoded))
	}
	require.Equal(t, before, []byte(result.Raw))
}
