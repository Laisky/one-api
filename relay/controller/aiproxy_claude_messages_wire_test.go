package controller

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/relaymode"
)

// TestAIProxyLibraryClaudeMessagesWireContract reproduces a protocol-conversion
// gap on the AIProxy library host, which routes Claude Messages by default. The
// same harness converts correctly for Chat Completions and Responses clients
// (library request in, converted response out) and for Claude Messages on
// Azure. For Claude Messages on AIProxy, the provider must receive the library
// request ({"query","libraryId"}) and the client must receive a Claude Messages
// response; today the provider receives an OpenAI chat body and the client
// receives an OpenAI chat.completion object.
func TestAIProxyLibraryClaudeMessagesWireContract(t *testing.T) {
	require.True(t, channeltype.IsEndpointSupported(relaymode.ClaudeMessages, channeltype.DefaultEndpointsForChannelType(channeltype.AIProxyLibrary)))
	const balance = int64(100_000)
	xaiVideoSetup(t, balance, false)
	upstream := newTariffUpstream(t, func(string) (string, string) { return "application/json", aiproxyLibraryReply })
	path, body := lyriaProtocolRequest("messages", false)
	c, writer, requestID := protocolContext(t, channeltype.AIProxyLibrary, "gpt-4o-mini", path, body, upstream.server.URL, balance, 1, false, nil)
	c.Set(ctxkey.Config, model.ChannelConfig{LibraryID: "library-fixture"})
	clearNativeSettledLedger(t, requestID)

	apiErr := RelayClaudeMessagesHelper(c)
	drainCriticalTasks(t)
	require.Nil(t, apiErr)
	require.EqualValues(t, 1, upstream.calls.Load())
	obs := upstream.next(t)
	require.Equal(t, "/api/library/ask", obs.Path)

	var wire map[string]any
	require.NoError(t, json.Unmarshal([]byte(obs.Body), &wire))
	require.Equal(t, "Create music", wire["query"], "the library API needs the last user message as query; got %s", obs.Body)
	require.Equal(t, "library-fixture", wire["libraryId"], "the library API needs the configured library ID; got %s", obs.Body)

	var reply map[string]any
	require.NoError(t, json.Unmarshal(writer.Body.Bytes(), &reply))
	require.Equal(t, "message", reply["type"], "a Claude Messages client must receive a Claude message; got %s", writer.Body.String())
	require.Equal(t, "assistant", reply["role"])
}
