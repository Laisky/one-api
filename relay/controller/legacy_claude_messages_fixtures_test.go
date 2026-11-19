package controller

import (
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/channeltype"
)

// legacyReplyByPath answers fixed JSON or stream bodies by request path.
// Parameters: chatPath is the generation path, json and stream the native
// non-stream and stream answers, and streamType the stream content type.
// Returns: a reply function for legacyClaudeFixture.reply.
func legacyReplyByPath(chatPath, json, stream, streamType string) func(bool, *http.Request) legacyWireReply {
	return func(isStream bool, r *http.Request) legacyWireReply {
		if r.URL.Path != chatPath {
			return legacyWireReply{Status: http.StatusNotFound, ContentType: "application/json", Body: `{"error":"unexpected path"}`}
		}
		if isStream {
			return legacyWireReply{ContentType: streamType, Body: stream}
		}
		return legacyWireReply{ContentType: "application/json", Body: json}
	}
}

// requireNoOpenAIChatFields asserts a provider request is not an OpenAI chat
// body. Parameters: t owns assertions and body the decoded request.
func requireNoOpenAIChatFields(t *testing.T, body map[string]any) {
	t.Helper()
	require.NotContains(t, body, "max_completion_tokens", "the provider must not receive an OpenAI chat body: %v", body)
}

// legacyClaudeFixtures returns every legacy provider fixture. Xunfei is not
// listed: it dials a hard-coded websocket host with its own dialer, which a
// fake HTTP provider cannot intercept.
func legacyClaudeFixtures() []legacyClaudeFixture {
	return []legacyClaudeFixture{
		{
			name: "aiproxy_library", channel: channeltype.AIProxyLibrary, model: "gpt-4o-mini", key: "aiproxy-fixture-key",
			config: model.ChannelConfig{LibraryID: "library-fixture"}, chatPath: "/api/library/ask", reservationOnly: true,
			reply: legacyReplyByPath("/api/library/ask",
				`{"success":true,"answer":"ack","documents":[]}`,
				"data:{\"content\":\"ack\",\"finish\":false,\"model\":\"gpt-4o-mini\"}\n\ndata:{\"content\":\"\",\"finish\":true,\"model\":\"gpt-4o-mini\"}\n\n",
				"text/event-stream"),
			wire: func(t *testing.T, stream bool, body map[string]any) {
				require.Equal(t, "Say ack", body["query"], "the library API takes the last user message as query: %v", body)
				require.Equal(t, "library-fixture", body["libraryId"])
				require.Equal(t, stream, body["stream"])
				require.NotContains(t, body, "messages")
			},
		},
		{
			name: "baidu", channel: channeltype.Baidu, model: "ERNIE-4.0-8K", key: "baidu-claude-client|baidu-claude-secret",
			chatPath: "/rpc/2.0/ai_custom/v1/wenxinworkshop/chat/completions_pro", receiptUsage: true,
			reply: func(stream bool, r *http.Request) legacyWireReply {
				if r.URL.Path == "/oauth/2.0/token" {
					return legacyWireReply{ContentType: "application/json", Body: `{"access_token":"baidu-access-fixture","expires_in":2592000}`}
				}
				return legacyReplyByPath("/rpc/2.0/ai_custom/v1/wenxinworkshop/chat/completions_pro",
					`{"id":"as-1","object":"chat.completion","created":1,"result":"ack","usage":{"prompt_tokens":11,"completion_tokens":2,"total_tokens":13}}`,
					"data: {\"id\":\"as-1\",\"result\":\"ack\",\"is_end\":false}\n\ndata: {\"id\":\"as-1\",\"result\":\"\",\"is_end\":true,\"usage\":{\"prompt_tokens\":11,\"completion_tokens\":2,\"total_tokens\":13}}\n\n",
					"text/event-stream")(stream, r)
			},
			wire: func(t *testing.T, stream bool, body map[string]any) {
				requireNoOpenAIChatFields(t, body)
				require.Equal(t, []any{map[string]any{"role": "user", "content": "Say ack"}}, body["messages"])
				require.EqualValues(t, 64, body["max_output_tokens"], "Baidu names the output limit max_output_tokens: %v", body)
				require.NotContains(t, body, "model")
			},
		},
		{
			name: "palm", channel: channeltype.PaLM, model: "chat-bison-001", key: "palm-fixture-key",
			chatPath:            "/v1beta2/models/chat-bison-001:generateMessage",
			noChatStreamControl: "PaLM answers a stream request with one application/json body, which the Chat Completions controller rejects as a stream error",
			reply: legacyReplyByPath("/v1beta2/models/chat-bison-001:generateMessage",
				`{"candidates":[{"author":"1","content":"ack"}]}`,
				`{"candidates":[{"author":"1","content":"ack"}]}`,
				"application/json"),
			wire: func(t *testing.T, _ bool, body map[string]any) {
				requireNoOpenAIChatFields(t, body)
				prompt, _ := body["prompt"].(map[string]any)
				messages, _ := prompt["messages"].([]any)
				require.Len(t, messages, 1, "PaLM nests messages under prompt, and an absent system prompt adds none: %v", body)
				require.Equal(t, "Say ack", messages[0].(map[string]any)["content"])
				require.EqualValues(t, 1, body["candidateCount"])
			},
		},
		{
			name: "zhipu_v3", channel: channeltype.Zhipu, model: "chatglm_turbo", key: "zhipu-id.zhipu-secret",
			chatPath: "/api/paas/v3/model-api/chatglm_turbo/invoke", streamPath: "/api/paas/v3/model-api/chatglm_turbo/sse-invoke", receiptUsage: true,
			reply: func(stream bool, r *http.Request) legacyWireReply {
				if stream {
					return legacyReplyByPath("/api/paas/v3/model-api/chatglm_turbo/sse-invoke", "",
						"data:ack\n\nmeta:{\"request_id\":\"r1\",\"task_id\":\"t1\",\"task_status\":\"SUCCESS\",\"usage\":{\"prompt_tokens\":11,\"completion_tokens\":2,\"total_tokens\":13}}\n\n",
						"text/event-stream")(stream, r)
				}
				return legacyReplyByPath("/api/paas/v3/model-api/chatglm_turbo/invoke",
					`{"code":200,"msg":"ok","success":true,"data":{"task_id":"t1","choices":[{"role":"assistant","content":"ack"}],"usage":{"prompt_tokens":11,"completion_tokens":2,"total_tokens":13}}}`,
					"", "")(stream, r)
			},
			wire: func(t *testing.T, _ bool, body map[string]any) {
				requireNoOpenAIChatFields(t, body)
				require.Equal(t, []any{map[string]any{"role": "user", "content": "Say ack"}}, body["prompt"], "Zhipu v3 sends the dialogue as prompt: %v", body)
				require.NotContains(t, body, "messages")
			},
		},
		{
			name: "zhipu_v4", channel: channeltype.Zhipu, model: "glm-4-flash", key: "zhipu-id.zhipu-secret",
			chatPath: "/api/paas/v4/chat/completions", receiptUsage: true,
			reply: legacyReplyByPath("/api/paas/v4/chat/completions", legacyOpenAIChatJSON("glm-4-flash"), legacyOpenAIChatSSE("glm-4-flash"), "text/event-stream"),
			wire: func(t *testing.T, stream bool, body map[string]any) {
				require.Equal(t, "glm-4-flash", body["model"])
				require.Equal(t, []any{map[string]any{"role": "user", "content": "Say ack"}}, body["messages"], "GLM v4 speaks OpenAI chat: %v", body)
			},
		},
		{
			name: "zai", channel: channeltype.Zai, model: "glm-4.5", key: "zai-fixture-key",
			chatPath: "/api/paas/v4/chat/completions", receiptUsage: true,
			reply: legacyReplyByPath("/api/paas/v4/chat/completions", legacyOpenAIChatJSON("glm-4.5"), legacyOpenAIChatSSE("glm-4.5"), "text/event-stream"),
			wire: func(t *testing.T, stream bool, body map[string]any) {
				require.Equal(t, "glm-4.5", body["model"])
				require.Equal(t, []any{map[string]any{"role": "user", "content": "Say ack"}}, body["messages"], "Z.AI speaks OpenAI chat: %v", body)
			},
		},
		{
			name: "ali", channel: channeltype.Ali, model: "qwen-plus", key: "ali-fixture-key",
			chatPath: "/api/v1/services/aigc/text-generation/generation", receiptUsage: true,
			reply: legacyReplyByPath("/api/v1/services/aigc/text-generation/generation",
				`{"output":{"choices":[{"finish_reason":"stop","message":{"role":"assistant","content":"ack"}}]},"usage":{"input_tokens":11,"output_tokens":2,"total_tokens":13},"request_id":"r1"}`,
				"data:{\"output\":{\"choices\":[{\"message\":{\"role\":\"assistant\",\"content\":\"ack\"},\"finish_reason\":\"null\"}]},\"usage\":{\"input_tokens\":11,\"output_tokens\":1}}\n\ndata:{\"output\":{\"choices\":[{\"message\":{\"role\":\"assistant\",\"content\":\"\"},\"finish_reason\":\"stop\"}]},\"usage\":{\"input_tokens\":11,\"output_tokens\":2}}\n\n",
				"text/event-stream"),
			wire: func(t *testing.T, _ bool, body map[string]any) {
				requireNoOpenAIChatFields(t, body)
				input, _ := body["input"].(map[string]any)
				require.Equal(t, []any{map[string]any{"role": "user", "content": "Say ack"}}, input["messages"], "DashScope nests messages under input: %v", body)
				require.Contains(t, body, "parameters")
			},
		},
		{
			name: "tencent", channel: channeltype.Tencent, model: "hunyuan-lite", key: "1|tencent-secret-id|tencent-secret-key",
			chatPath: "/",
			reply: legacyReplyByPath("/",
				`{"Response":{"Choices":[{"FinishReason":"stop","Message":{"Role":"assistant","Content":"ack"}}],"Usage":{"PromptTokens":11,"CompletionTokens":2,"TotalTokens":13},"RequestId":"r1"}}`,
				"data:{\"Choices\":[{\"Delta\":{\"Role\":\"assistant\",\"Content\":\"ack\"},\"FinishReason\":\"\"}]}\n\ndata:{\"Choices\":[{\"Delta\":{\"Content\":\"\"},\"FinishReason\":\"stop\"}]}\n\n",
				"text/event-stream"),
			wire: func(t *testing.T, stream bool, body map[string]any) {
				requireNoOpenAIChatFields(t, body)
				require.Equal(t, "hunyuan-lite", body["Model"], "Hunyuan uses PascalCase fields: %v", body)
				require.Equal(t, []any{map[string]any{"Role": "user", "Content": "Say ack"}}, body["Messages"])
				require.Equal(t, stream, body["Stream"])
			},
		},
		{
			name: "ollama", channel: channeltype.Ollama, model: "llama3", key: "ollama-fixture-key",
			chatPath: "/api/chat", receiptUsage: true,
			reply: legacyReplyByPath("/api/chat",
				`{"model":"llama3","message":{"role":"assistant","content":"ack"},"done":true,"prompt_eval_count":11,"eval_count":2}`,
				"{\"model\":\"llama3\",\"message\":{\"role\":\"assistant\",\"content\":\"ack\"},\"done\":false}\n{\"model\":\"llama3\",\"message\":{\"role\":\"assistant\",\"content\":\"\"},\"done\":true,\"prompt_eval_count\":11,\"eval_count\":2}\n",
				"application/x-ndjson"),
			wire: func(t *testing.T, stream bool, body map[string]any) {
				requireNoOpenAIChatFields(t, body)
				options, _ := body["options"].(map[string]any)
				require.EqualValues(t, 64, options["num_predict"], "Ollama takes the output limit as options.num_predict: %v", body)
				messages, _ := body["messages"].([]any)
				require.Len(t, messages, 1, "an absent system prompt adds no message: %v", body)
				require.Equal(t, "user", messages[0].(map[string]any)["role"])
				require.Equal(t, stream, body["stream"])
			},
		},
		{
			name: "coze", channel: channeltype.Coze, model: "bot-fixture", key: "coze-fixture-key",
			config: model.ChannelConfig{UserID: "coze-user"}, chatPath: "/open_api/v2/chat",
			reply: legacyReplyByPath("/open_api/v2/chat",
				`{"conversation_id":"c1","code":0,"msg":"success","messages":[{"role":"assistant","type":"answer","content":"ack","content_type":"text"}]}`,
				"data:{\"event\":\"message\",\"message\":{\"role\":\"assistant\",\"type\":\"answer\",\"content\":\"ack\",\"content_type\":\"text\"},\"is_finish\":false,\"index\":0,\"conversation_id\":\"c1\"}\n\ndata:{\"event\":\"done\"}\n\n",
				"text/event-stream"),
			wire: func(t *testing.T, stream bool, body map[string]any) {
				requireNoOpenAIChatFields(t, body)
				require.Equal(t, "fixture", body["bot_id"], "Coze addresses a bot: %v", body)
				require.Equal(t, "Say ack", body["query"])
				require.Equal(t, "coze-user", body["user"])
				require.Empty(t, body["chat_history"], "an absent system prompt adds no history turn: %v", body)
			},
		},
		{
			name: "cohere", channel: channeltype.Cohere, model: "command-r", key: "cohere-fixture-key",
			chatPath: "/v1/chat", receiptUsage: true,
			reply: legacyReplyByPath("/v1/chat",
				`{"response_id":"r1","text":"ack","generation_id":"g1","finish_reason":"COMPLETE","meta":{"tokens":{"input_tokens":11,"output_tokens":2}}}`,
				"{\"is_finished\":false,\"event_type\":\"text-generation\",\"text\":\"ack\"}\n{\"is_finished\":true,\"event_type\":\"stream-end\",\"finish_reason\":\"COMPLETE\",\"response\":{\"response_id\":\"r1\",\"finish_reason\":\"COMPLETE\",\"meta\":{\"tokens\":{\"input_tokens\":11,\"output_tokens\":2}}}}\n",
				"application/stream+json"),
			wire: func(t *testing.T, stream bool, body map[string]any) {
				requireNoOpenAIChatFields(t, body)
				require.Equal(t, "Say ack", body["message"], "Cohere chat takes the last user turn as message: %v", body)
				require.NotContains(t, body, "messages")
			},
		},
		legacyReplicateFixture(),
	}
}

// legacyReplicateFixture is the Replicate prediction flow: create (201),
// poll until succeeded, then read the prediction's SSE stream when streaming.
func legacyReplicateFixture() legacyClaudeFixture {
	const modelName = "meta/meta-llama-3-70b-instruct"
	createPath := "/v1/models/" + modelName + "/predictions"
	return legacyClaudeFixture{
		name: "replicate", channel: channeltype.Replicate, model: modelName, key: "replicate-fixture-key",
		chatPath: createPath,
		reply: func(_ bool, r *http.Request) legacyWireReply {
			switch r.URL.Path {
			case createPath:
				return legacyWireReply{Status: http.StatusCreated, ContentType: "application/json",
					Body: `{"id":"p1","status":"starting","urls":{"get":"https://example.com/v1/predictions/p1"}}`}
			case "/v1/predictions/p1":
				return legacyWireReply{ContentType: "application/json",
					Body: `{"id":"p1","status":"succeeded","output":["a","ck"],"urls":{"get":"https://example.com/v1/predictions/p1","stream":"https://example.com/v1/streams/p1"}}`}
			case "/v1/streams/p1":
				return legacyWireReply{ContentType: "text/event-stream", Body: "event: output\ndata: ack\n\nevent: done\ndata: {}\n\n"}
			}
			return legacyWireReply{Status: http.StatusNotFound, ContentType: "application/json", Body: `{"error":"unexpected path"}`}
		},
		wire: func(t *testing.T, _ bool, body map[string]any) {
			requireNoOpenAIChatFields(t, body)
			input, _ := body["input"].(map[string]any)
			prompt, _ := input["prompt"].(string)
			require.True(t, strings.Contains(prompt, "Say ack"), "Replicate takes a flattened prompt: %v", body)
			require.NotContains(t, body, "messages")
		},
	}
}

// legacyOpenAIChatJSON is an OpenAI-wire chat completion answering "ack".
func legacyOpenAIChatJSON(modelName string) string {
	return `{"id":"chatcmpl-1","object":"chat.completion","created":1,"model":"` + modelName + `","choices":[{"index":0,"finish_reason":"stop","message":{"role":"assistant","content":"ack"}}],"usage":{"prompt_tokens":11,"completion_tokens":2,"total_tokens":13}}`
}

// legacyOpenAIChatSSE is an OpenAI-wire chat stream answering "ack" with a
// final usage chunk.
func legacyOpenAIChatSSE(modelName string) string {
	return `data: {"id":"chatcmpl-1","object":"chat.completion.chunk","created":1,"model":"` + modelName + `","choices":[{"index":0,"delta":{"role":"assistant","content":"ack"},"finish_reason":null}]}` + "\n\n" +
		`data: {"id":"chatcmpl-1","object":"chat.completion.chunk","created":1,"model":"` + modelName + `","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}` + "\n\n" +
		`data: {"id":"chatcmpl-1","object":"chat.completion.chunk","created":1,"model":"` + modelName + `","choices":[],"usage":{"prompt_tokens":11,"completion_tokens":2,"total_tokens":13}}` + "\n\n" +
		"data: [DONE]\n\n"
}
