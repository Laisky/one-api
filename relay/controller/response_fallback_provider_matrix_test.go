package controller

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws/protocol/eventstream"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/adaptor/vertexai"
	"github.com/Laisky/one-api/relay/channeltype"
	metalib "github.com/Laisky/one-api/relay/meta"
	relaymodel "github.com/Laisky/one-api/relay/model"
)

// providerMatrixBalance is the finite user and token balance every matrix run
// starts from, large enough that no reservation is ever refused.
const providerMatrixBalance = int64(1_000_000)

// providerMatrixClaudeModel is a real Claude model name that every Claude
// platform in the matrix (first-party, Foundry, Bedrock, Vertex) catalogs, so
// each adaptor resolves its Claude route instead of rejecting the model.
const providerMatrixClaudeModel = "claude-opus-4-6"

// providerMatrixVertexProject and providerMatrixVertexRegion select the Vertex
// AI rawPredict route; providerMatrixVertexToken is the synthetic access token
// seeded into the adaptor's token cache so no Google OAuth exchange runs.
const (
	providerMatrixVertexProject = "matrix-project"
	providerMatrixVertexRegion  = "us-east5"
	providerMatrixVertexToken   = "vertex-matrix-token"
)

// providerMatrixFamily is the upstream wire format a fallback provider speaks.
type providerMatrixFamily int

const (
	// providerMatrixChat is OpenAI Chat Completions JSON or SSE.
	providerMatrixChat providerMatrixFamily = iota
	// providerMatrixClaude is Anthropic Claude Messages JSON or SSE.
	providerMatrixClaude
	// providerMatrixGemini is native Gemini generateContent JSON or SSE.
	providerMatrixGemini
)

// providerMatrixProvider describes one provider that takes the Responses
// Chat/Claude fallback: its channel type, upstream model, channel config, the
// transport it is reached through and the upstream route it must call.
type providerMatrixProvider struct {
	name    string
	channel int
	actual  string
	family  providerMatrixFamily
	config  model.ChannelConfig
	// bedrock marks the AWS SDK transport: the SDK dials a plain HTTP endpoint
	// selected by AWS_ENDPOINT_URL instead of using client.HTTPClient, and
	// streams arrive as AWS EventStream frames.
	bedrock bool
	// vertexToken seeds the Vertex AI access-token cache for the fixture channel.
	vertexToken string
	// path returns the anchored regular expression the upstream request path
	// must match for the given stream flag.
	path func(stream bool) string
	// query, when set, returns a substring the upstream raw query must contain.
	query func(stream bool) string
	// auth asserts the provider-specific credential the upstream received.
	auth func(t *testing.T, header http.Header)
	// streamInBody reports that the provider selects SSE through the body's
	// "stream" flag, so a streaming request must carry "stream":true. Bedrock
	// and Gemini select streaming by endpoint and must not carry the flag.
	streamInBody bool
}

// providerMatrixLiteralPath returns a path matcher that accepts exactly the
// non-stream path, or the stream path when streaming. Parameters: plain and
// streamed are the literal expected paths. Returns the matcher function.
func providerMatrixLiteralPath(plain, streamed string) func(bool) string {
	return func(stream bool) string {
		if stream {
			return "^" + regexp.QuoteMeta(streamed) + "$"
		}
		return "^" + regexp.QuoteMeta(plain) + "$"
	}
}

// providerMatrixHeaderEquals returns an auth assertion that header key carries
// exactly want. Parameters: key is the header name and want its value.
func providerMatrixHeaderEquals(key, want string) func(*testing.T, http.Header) {
	return func(t *testing.T, header http.Header) {
		t.Helper()
		require.Equal(t, want, header.Get(key), "upstream credential header %s", key)
	}
}

// providerMatrixProviders returns every provider family that takes the
// Responses fallback, in a stable order. The Claude platforms share one model
// so their lowering differs only by platform; DeepSeek uses a DeepSeek-family
// name that is not one of the natively Responses-capable Flash/Pro models.
func providerMatrixProviders() []providerMatrixProvider {
	geminiModel := "gemini-2.5-flash"
	return []providerMatrixProvider{
		{
			name: "openai_compatible", channel: channeltype.OpenAICompatible, actual: "codex-fallback-fixture", family: providerMatrixChat,
			path: providerMatrixLiteralPath("/v1/chat/completions", "/v1/chat/completions"),
			auth: providerMatrixHeaderEquals("Authorization", "Bearer upstream-fixture-key"), streamInBody: true,
		},
		{
			name: "deepseek", channel: channeltype.DeepSeek, actual: "deepseek-matrix-fixture", family: providerMatrixChat,
			path: providerMatrixLiteralPath("/v1/chat/completions", "/v1/chat/completions"),
			auth: providerMatrixHeaderEquals("Authorization", "Bearer upstream-fixture-key"), streamInBody: true,
		},
		{
			name: "anthropic", channel: channeltype.Anthropic, actual: providerMatrixClaudeModel, family: providerMatrixClaude,
			path: providerMatrixLiteralPath("/v1/messages", "/v1/messages"),
			auth: providerMatrixHeaderEquals("x-api-key", "upstream-fixture-key"), streamInBody: true,
		},
		{
			name: "azure_foundry_claude", channel: channeltype.Azure, actual: providerMatrixClaudeModel, family: providerMatrixClaude,
			path: providerMatrixLiteralPath("/anthropic/v1/messages", "/anthropic/v1/messages"),
			auth: providerMatrixHeaderEquals("x-api-key", "upstream-fixture-key"), streamInBody: true,
		},
		{
			name: "aws_bedrock_claude", channel: channeltype.AwsClaude, actual: providerMatrixClaudeModel, family: providerMatrixClaude, bedrock: true,
			config: model.ChannelConfig{Region: "us-east-1", AK: "matrix-access", SK: "matrix-secret"},
			path: func(stream bool) string {
				action := "invoke"
				if stream {
					action = "invoke-with-response-stream"
				}
				// The SDK may prefix a cross-region inference profile (us., global.).
				return `^/model/([a-z]+\.)?anthropic\.claude-opus-4-6-v1/` + regexp.QuoteMeta(action) + `$`
			},
			auth: func(t *testing.T, header http.Header) {
				t.Helper()
				require.True(t, strings.HasPrefix(header.Get("Authorization"), "AWS4-HMAC-SHA256 Credential=matrix-access/"),
					"Bedrock requests are SigV4-signed with the channel access key")
			},
		},
		{
			name: "vertex_claude", channel: channeltype.VertextAI, actual: providerMatrixClaudeModel, family: providerMatrixClaude,
			config:      model.ChannelConfig{VertexAIProjectID: providerMatrixVertexProject, Region: providerMatrixVertexRegion},
			vertexToken: providerMatrixVertexToken,
			// Streams use streamRawPredict and keep "stream":true in the body,
			// exactly as Anthropic's own Vertex client builds them
			// (anthropic-sdk-python lib/vertex/_client.py, _prepare_options);
			// rawPredict without the flag returns one non-streaming message.
			path: providerMatrixLiteralPath(
				"/v1/projects/"+providerMatrixVertexProject+"/locations/"+providerMatrixVertexRegion+"/publishers/anthropic/models/"+providerMatrixClaudeModel+":rawPredict",
				"/v1/projects/"+providerMatrixVertexProject+"/locations/"+providerMatrixVertexRegion+"/publishers/anthropic/models/"+providerMatrixClaudeModel+":streamRawPredict"),
			auth:         providerMatrixHeaderEquals("Authorization", "Bearer "+providerMatrixVertexToken),
			streamInBody: true,
		},
		{
			name: "gemini", channel: channeltype.Gemini, actual: geminiModel, family: providerMatrixGemini,
			path: providerMatrixLiteralPath("/v1beta/models/"+geminiModel+":generateContent", "/v1beta/models/"+geminiModel+":streamGenerateContent"),
			query: func(stream bool) string {
				if stream {
					return "alt=sse"
				}
				return ""
			},
			auth: providerMatrixHeaderEquals("x-goog-api-key", "upstream-fixture-key"),
		},
	}
}

// providerMatrixWireChannel returns the channel type whose fixture framing and
// call IDs the provider shares: Anthropic (toolu_ IDs) for every Claude
// platform, OpenAICompatible (call_ IDs) for Chat and Gemini.
func providerMatrixWireChannel(p providerMatrixProvider) int {
	if p.family == providerMatrixClaude {
		return channeltype.Anthropic
	}
	return channeltype.OpenAICompatible
}

// providerMatrixReply is a fully encoded fake upstream reply.
type providerMatrixReply struct {
	contentType string
	body        []byte
}

// buildProviderMatrixReply encodes the provider's fake reply on the test
// goroutine, so the HTTP handler never asserts off it. Every reply carries the
// assistant text "Working.", the codexFallbackUpstreamCalls tool calls
// (namespaced MCP, collaboration, connector, root, custom apply_patch and
// tool_search) and usage of 11 prompt and 7 completion tokens. Parameters: t
// owns assertions, p selects the wire format and stream selects streaming.
// Returns the encoded reply.
func buildProviderMatrixReply(t *testing.T, p providerMatrixProvider, stream bool) providerMatrixReply {
	t.Helper()
	if p.family == providerMatrixGemini {
		return buildProviderMatrixGeminiReply(stream)
	}
	recorder := httptest.NewRecorder()
	writeCodexFallbackReply(recorder, providerMatrixWireChannel(p), stream)
	reply := providerMatrixReply{contentType: recorder.Header().Get("Content-Type"), body: recorder.Body.Bytes()}
	if p.bedrock && stream {
		reply = providerMatrixBedrockFrames(t, reply.body)
	}
	return reply
}

// providerMatrixBedrockFrames re-encodes Claude SSE events as the AWS
// EventStream chunk frames InvokeModelWithResponseStream returns: each frame's
// JSON payload carries one Claude event in its "bytes" field. Parameters: t
// owns assertions and sse is the Claude SSE body. Returns the framed reply.
func providerMatrixBedrockFrames(t *testing.T, sse []byte) providerMatrixReply {
	t.Helper()
	var out bytes.Buffer
	for _, event := range parseSSEEvents(string(sse)) {
		if event.data == "" {
			continue
		}
		payload, err := json.Marshal(map[string][]byte{"bytes": []byte(event.data)})
		require.NoError(t, err)
		var headers eventstream.Headers
		headers.Set(":message-type", eventstream.StringValue("event"))
		headers.Set(":event-type", eventstream.StringValue("chunk"))
		headers.Set(":content-type", eventstream.StringValue("application/json"))
		require.NoError(t, eventstream.NewEncoder().Encode(&out, eventstream.Message{Headers: headers, Payload: payload}))
	}
	return providerMatrixReply{contentType: "application/vnd.amazon.eventstream", body: out.Bytes()}
}

// buildProviderMatrixGeminiReply encodes a native Gemini reply carrying the
// fixture text, one functionCall part per codexFallbackUpstreamCalls entry and
// usageMetadata. Gemini function calls carry no IDs; the adaptor mints them.
// Parameters: stream selects alt=sse chunks. Returns the encoded reply.
func buildProviderMatrixGeminiReply(stream bool) providerMatrixReply {
	upstream := codexFallbackUpstreamCalls(channeltype.OpenAICompatible)
	calls := make([]string, 0, len(upstream))
	for _, call := range upstream {
		calls = append(calls, fmt.Sprintf(`{"functionCall":{"name":%q,"args":%s}}`, call.name, call.arguments))
	}
	const usage = `"usageMetadata":{"promptTokenCount":11,"candidatesTokenCount":7,"totalTokenCount":18},"modelVersion":"gemini-matrix-fixture"`
	if !stream {
		body := fmt.Sprintf(`{"candidates":[{"content":{"role":"model","parts":[{"text":"Working."},%s]},"finishReason":"STOP","index":0}],%s}`, strings.Join(calls, ","), usage)
		return providerMatrixReply{contentType: "application/json", body: []byte(body)}
	}
	chunks := []string{
		`{"candidates":[{"content":{"role":"model","parts":[{"text":"Working."}]},"index":0}],"modelVersion":"gemini-matrix-fixture"}`,
		fmt.Sprintf(`{"candidates":[{"content":{"role":"model","parts":[%s]},"finishReason":"STOP","index":0}],%s}`, strings.Join(calls, ","), usage),
	}
	var body strings.Builder
	for _, chunk := range chunks {
		fmt.Fprintf(&body, "data: %s\r\n\r\n", chunk)
	}
	return providerMatrixReply{contentType: "text/event-stream", body: []byte(body.String())}
}

// providerMatrixObservation is what the fake provider received: the shared
// Codex observation (path, body, balances held at dispatch) plus the raw query
// and request headers for route and credential assertions.
type providerMatrixObservation struct {
	codexFallbackObservation
	Query  string
	Header http.Header
}

// providerMatrixResult is the observable outcome of one fallback relay.
type providerMatrixResult struct {
	c         *gin.Context
	recorder  *httptest.ResponseRecorder
	requestID string
	apiErr    *relaymodel.ErrorWithStatusCode
	got       providerMatrixObservation
	observed  bool
	calls     int32
}

// providerMatrixOptions customizes one matrix run.
type providerMatrixOptions struct {
	// prepare adjusts the request context (for example channel tooling)
	// before relaying.
	prepare func(*gin.Context)
	// reply replaces the default Codex tool-call reply of the provider.
	reply *providerMatrixReply
}

// runProviderMatrix relays one Responses request through provider p's
// fallback against a fresh fake upstream and drains post-billing work.
// Parameters: t owns fixtures, p is the provider, stream selects the fake
// reply framing, body is the client Responses request and opts optionally
// adjusts the context or replaces the reply. Returns the result.
func runProviderMatrix(t *testing.T, p providerMatrixProvider, stream bool, body string, opts providerMatrixOptions) providerMatrixResult {
	t.Helper()
	xaiVideoSetup(t, providerMatrixBalance, false)
	reply := buildProviderMatrixReply(t, p, stream)
	if opts.reply != nil {
		reply = *opts.reply
	}
	observed := make(chan providerMatrixObservation, 1)
	var calls atomic.Int32
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		got := providerMatrixObservation{Query: r.URL.RawQuery, Header: r.Header.Clone()}
		got.Path = r.URL.Path
		got.Body, got.Err = io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
		var user model.User
		var token model.Token
		if got.Err == nil {
			got.Err = model.DB.First(&user, fallbackUserID).Error
		}
		if got.Err == nil {
			got.Err = model.DB.First(&token, fallbackTokenID).Error
		}
		got.User, got.Token = user.Quota, token.RemainQuota
		select {
		case observed <- got:
		default:
		}
		w.Header().Set("Content-Type", reply.contentType)
		if p.bedrock {
			w.Header().Set("X-Amzn-Bedrock-Content-Type", "application/json")
		}
		_, _ = w.Write(reply.body)
	})

	var base string
	if p.bedrock {
		// The AWS SDK owns its HTTP client; route it to a plain loopback server
		// and keep any developer AWS profile out of the run.
		server := httptest.NewServer(handler)
		t.Cleanup(server.Close)
		dir := t.TempDir()
		t.Setenv("AWS_ENDPOINT_URL", server.URL)
		t.Setenv("AWS_MAX_ATTEMPTS", "1")
		t.Setenv("AWS_CONFIG_FILE", filepath.Join(dir, "config"))
		t.Setenv("AWS_SHARED_CREDENTIALS_FILE", filepath.Join(dir, "credentials"))
		base = server.URL
	} else {
		server := httptest.NewTLSServer(handler)
		t.Cleanup(server.Close)
		previous := client.HTTPClient
		client.HTTPClient = server.Client()
		t.Cleanup(func() { client.HTTPClient = previous })
		base = server.URL
	}
	if p.vertexToken != "" {
		key := fmt.Sprintf("vertexai-token-%d", fallbackChannelID)
		vertexai.Cache.Set(key, p.vertexToken, time.Minute)
		t.Cleanup(func() { vertexai.Cache.Delete(key) })
	}

	c, recorder, requestID := protocolContext(t, p.channel, p.actual, "/v1/responses", body, base, providerMatrixBalance, 1, false,
		&model.ModelConfigLocal{Ratio: 1, CompletionRatio: 2})
	c.Set(ctxkey.Config, p.config)
	if opts.prepare != nil {
		opts.prepare(c)
	}
	responseNamespaceCleanLedger(t, requestID)
	apiErr := RelayResponseAPIHelper(c)
	drainCriticalTasks(t)
	result := providerMatrixResult{c: c, recorder: recorder, requestID: requestID, apiErr: apiErr, calls: calls.Load()}
	select {
	case result.got = <-observed:
		result.observed = true
	default:
	}
	return result
}

// requireProviderMatrixDispatched asserts a successful fallback relay that
// reached the provider exactly once: no error, HTTP 200, the fallback flag set
// on the mapped request and no native Responses passthrough for it.
// Parameters: t owns assertions and result is the relay outcome.
func requireProviderMatrixDispatched(t *testing.T, result providerMatrixResult) {
	t.Helper()
	require.Nil(t, result.apiErr, "fallback must emulate Codex tools instead of rejecting them: %v", result.apiErr)
	require.Equal(t, http.StatusOK, result.recorder.Code, result.recorder.Body.String())
	require.EqualValues(t, 1, result.calls, "exactly one upstream dispatch")
	require.True(t, result.observed, "the provider observed the request")
	require.NoError(t, result.got.Err)
	meta := metalib.GetByContext(result.c)
	require.True(t, meta.ResponseAPIFallback, "the request must take the Chat/Claude fallback")
	require.False(t, supportsNativeResponseAPI(meta), "the mapped channel must not support native Responses")
}

// requireProviderMatrixRoute asserts assertion (1): the request reached the
// provider's own upstream route with its own credential scheme and asked for
// the response mode the client requested. Parameters: t owns assertions, p is
// the provider, stream the mode and got the observation.
func requireProviderMatrixRoute(t *testing.T, p providerMatrixProvider, stream bool, got providerMatrixObservation) {
	t.Helper()
	require.Regexp(t, p.path(stream), got.Path, "upstream path for %s", p.name)
	if p.query != nil {
		if want := p.query(stream); want != "" {
			require.Contains(t, got.Query, want)
		}
	}
	p.auth(t, got.Header)
	var envelope struct {
		Stream *bool `json:"stream"`
	}
	require.NoError(t, json.Unmarshal(got.Body, &envelope))
	if !p.streamInBody {
		require.Nil(t, envelope.Stream, "%s selects streaming by endpoint, not by a body flag", p.name)
		return
	}
	require.Equal(t, stream, envelope.Stream != nil && *envelope.Stream, "%s must receive \"stream\":%t in the request body", p.name, stream)
}

// TestResponseFallbackProviderMatrix relays the same synthetic Codex
// transcript (tool search discovery, a namespaced MCP call, a custom
// apply_patch call, the multi-agent turn and an agent message) through every
// provider family that takes the Responses fallback, as JSON and as SSE. Per
// provider and mode it proves: (1) the request reached that provider's route
// and credential scheme; (2) the upstream received the flattened tool catalog
// and the lowered history; (3) the client reply passes the Responses
// wire-conformance validator; (4) upstream tool calls render back with their
// Codex identities; and (5) one token-only settlement of 11 prompt + 7*2
// completion quota, held before dispatch and charged exactly once.
func TestResponseFallbackProviderMatrix(t *testing.T) {
	for _, p := range providerMatrixProviders() {
		for _, stream := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/stream=%t", p.name, stream), func(t *testing.T) {
				result := runProviderMatrix(t, p, stream, codexFallbackRequestBody(stream), providerMatrixOptions{})
				requireProviderMatrixDispatched(t, result)
				requireProviderMatrixRoute(t, p, stream, result.got)
				requireProviderMatrixUpstream(t, p, result.got)
				requireResponsesRecorderConformance(t, result.recorder)
				output := requireProviderMatrixReplyEnvelope(t, result.recorder, stream)
				requireProviderMatrixItems(t, p, output)
				if stream {
					requireProviderMatrixStreamEvents(t, p, result.recorder.Body.String())
				}
				requireProviderMatrixSettlement(t, result)
			})
		}
	}
}
