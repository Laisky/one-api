package controller

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/channeltype"
)

// TestJinaOCRClaudeMessagesWireContract drives Claude Messages requests to the
// Jina OCR chat endpoint, which answers in the OpenAI Chat wire format with a
// usage receipt. Jina bills its own verified receipt, so the Claude client
// must still receive the converted Claude message even though the adaptor
// returns usage to the controller.
func TestJinaOCRClaudeMessagesWireContract(t *testing.T) {
	const modelName = "jina-ocr-v1"
	runLegacyClaudeContract(t, legacyClaudeFixture{
		name: "jina_ocr", channel: channeltype.Jina, model: modelName, key: "jina-fixture-key",
		chatPath: "/chat/completions", receiptUsage: true,
		reply: legacyReplyByPath("/v1/chat/completions", legacyOpenAIChatJSON(modelName), legacyOpenAIChatSSE(modelName), "text/event-stream"),
		wire: func(t *testing.T, _ bool, body map[string]any) {
			require.Equal(t, modelName, body["model"])
			require.Equal(t, []any{map[string]any{"role": "user", "content": "Say ack"}}, body["messages"])
		},
	})
}
