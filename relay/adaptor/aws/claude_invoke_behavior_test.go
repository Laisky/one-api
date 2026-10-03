package aws

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/protocol/eventstream"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/ctxkey"
	channelmodel "github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/meta"
	relaymodel "github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/relaymode"
)

const claudeInvokeReceipt = `{"id":"msg_test","type":"message","role":"assistant","model":"claude-3-haiku-20240307","content":[{"type":"text","text":"Hello"}],"stop_reason":"end_turn","future_field":{"id":9007199254740993},"usage":{"input_tokens":21,"output_tokens":50,"cache_read_input_tokens":1000,"cache_creation_input_tokens":300,"cache_creation":{"ephemeral_5m_input_tokens":100,"ephemeral_1h_input_tokens":200}}}`
const claudeInvokeStart = `{"type":"message_start","message":{"id":"msg_test","type":"message","role":"assistant","model":"claude-3-haiku-20240307","content":[],"usage":{"input_tokens":21,"output_tokens":1,"cache_read_input_tokens":1000,"cache_creation":{"ephemeral_5m_input_tokens":100,"ephemeral_1h_input_tokens":200}}}}`
const claudeInvokeTextStart = `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`
const claudeInvokeText = `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello"}}`
const claudeInvokeTextStop = `{"type":"content_block_stop","index":0}`
const claudeInvokeDelta = `{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"input_tokens":21,"output_tokens":50,"cache_read_input_tokens":1000,"cache_creation":{"ephemeral_5m_input_tokens":100,"ephemeral_1h_input_tokens":200}}}`
const claudeInvokeStop = `{"type":"message_stop"}`

// claudeInvokeRecorder supplies CloseNotify for the legacy handler's gin.Stream call.
type claudeInvokeRecorder struct {
	*httptest.ResponseRecorder
	closed chan bool
}

// CloseNotify returns the test connection's notification channel without closing it.
func (r *claudeInvokeRecorder) CloseNotify() <-chan bool { return r.closed }

// claudeInvokeTransport captures signed SDK requests and returns a deterministic response.
type claudeInvokeTransport struct {
	mu     sync.Mutex
	bodies []string
	paths  []string
	body   []byte
	stream bool
}

// RoundTrip records one HTTP request and returns fixture bytes without external traffic.
func (r *claudeInvokeTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	body, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, errors.Wrap(err, "read SDK request in test")
	}
	r.mu.Lock()
	r.bodies = append(r.bodies, string(body))
	r.paths = append(r.paths, req.URL.Path)
	r.mu.Unlock()
	contentType := "application/json"
	if r.stream {
		contentType = "application/vnd.amazon.eventstream"
	}
	return &http.Response{StatusCode: http.StatusOK, Header: http.Header{
		"Content-Type": {contentType}, "X-Amzn-Bedrock-Content-Type": {"application/json"},
	}, Body: io.NopCloser(bytes.NewReader(r.body)), Request: req}, nil
}

// claudeInvokeHarness binds the real AWS adapter and SDK to an isolated HTTP fixture.
type claudeInvokeHarness struct {
	adaptor   *Adaptor
	context   *gin.Context
	recorder  *claudeInvokeRecorder
	transport *claudeInvokeTransport
	meta      *meta.Meta
}

// newClaudeInvokeHarness creates a request-local adapter; it returns no shared mutable fixtures.
func newClaudeInvokeHarness(t *testing.T, stream bool, body []byte, region string) *claudeInvokeHarness {
	t.Helper()
	r := &claudeInvokeRecorder{ResponseRecorder: httptest.NewRecorder(), closed: make(chan bool)}
	c, _ := gin.CreateTestContext(r)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	gmw.SetLogger(c, glog.Shared.Named("claude-invoke-behavior"))
	transport := &claudeInvokeTransport{body: body, stream: stream}
	client := bedrockruntime.New(bedrockruntime.Options{
		Region: region, Credentials: credentials.NewStaticCredentialsProvider("test-access-key", "test-secret", ""),
		HTTPClient: &http.Client{Transport: transport}, Retryer: aws.NopRetryer{},
	})
	return &claudeInvokeHarness{adaptor: &Adaptor{AwsClient: client}, context: c, recorder: r, transport: transport,
		meta: &meta.Meta{ActualModelName: "claude-3-haiku-20240307", IsStream: stream}}
}

// prepareChat exercises public request conversion and DoRequest before SDK invocation.
func (h *claudeInvokeHarness) prepareChat(t *testing.T) {
	t.Helper()
	request := &relaymodel.GeneralOpenAIRequest{Model: h.meta.ActualModelName, MaxTokens: 256, Stream: h.meta.IsStream,
		Messages: []relaymodel.Message{{Role: "user", Content: "Hello"}}}
	converted, err := h.adaptor.ConvertRequest(h.context, relaymode.ChatCompletions, request)
	require.NoError(t, err)
	body, err := json.Marshal(converted)
	require.NoError(t, err)
	_, err = h.adaptor.DoRequest(h.context, h.meta, bytes.NewReader(body))
	require.NoError(t, err)
}

// prepareNative passes the controller-prepared body separately from its typed billing view.
func (h *claudeInvokeHarness) prepareNative(t *testing.T, body string) {
	t.Helper()
	var request relaymodel.ClaudeRequest
	require.NoError(t, json.Unmarshal([]byte(body), &request))
	_, err := h.adaptor.ConvertClaudeRequest(h.context, &request)
	require.NoError(t, err)
	_, err = h.adaptor.DoRequest(h.context, h.meta, strings.NewReader(body))
	require.NoError(t, err)
}

// claudeEventFrames encodes fixture JSON with the AWS SDK's binary EventStream encoder.
func claudeEventFrames(t *testing.T, events ...string) []byte {
	t.Helper()
	var out bytes.Buffer
	encoder := eventstream.NewEncoder()
	for _, event := range events {
		payload, err := json.Marshal(map[string][]byte{"bytes": []byte(event)})
		require.NoError(t, err)
		var headers eventstream.Headers
		headers.Set(":message-type", eventstream.StringValue("event"))
		headers.Set(":event-type", eventstream.StringValue("chunk"))
		headers.Set(":content-type", eventstream.StringValue("application/json"))
		require.NoError(t, encoder.Encode(&out, eventstream.Message{Headers: headers, Payload: payload}))
	}
	return out.Bytes()
}

// claudeExceptionFrame produces an SDK-level exception, not an Anthropic JSON error event.
func claudeExceptionFrame(t *testing.T) []byte {
	t.Helper()
	var out bytes.Buffer
	var headers eventstream.Headers
	headers.Set(":message-type", eventstream.StringValue("exception"))
	headers.Set(":exception-type", eventstream.StringValue("internalServerException"))
	headers.Set(":content-type", eventstream.StringValue("application/json"))
	require.NoError(t, eventstream.NewEncoder().Encode(&out, eventstream.Message{Headers: headers, Payload: []byte(`{"message":"synthetic stream failure"}`)}))
	return out.Bytes()
}

// TestClaudeInvokePreservesNativeFields checks the final SDK payload and native response.
func TestClaudeInvokePreservesNativeFields(t *testing.T) {
	t.Parallel()
	h := newClaudeInvokeHarness(t, false, []byte(claudeInvokeReceipt), "test-region-1")
	body := `{"model":"claude-3-haiku-20240307","stream":false,"max_tokens":256,"system":[{"type":"text","text":"policy","cache_control":{"type":"ephemeral","ttl":"1h"}}],"messages":[{"role":"user","content":[{"type":"text","text":"Hello","cache_control":{"type":"ephemeral"}}]}],"output_config":{"effort":"medium"},"anthropic_beta":["effort-2025-11-24"],"tools":[{"name":"lookup","input_schema":{"type":"object","additionalProperties":false,"properties":{}},"cache_control":{"type":"ephemeral"}}],"future_field":{"id":9007199254740993}}`
	h.prepareNative(t, body)
	usage, responseErr := h.adaptor.DoResponse(h.context, nil, h.meta)
	require.Nil(t, responseErr)
	require.NotNil(t, usage)
	require.Len(t, h.transport.bodies, 1)
	var actual, expected map[string]json.RawMessage
	require.NoError(t, json.Unmarshal([]byte(h.transport.bodies[0]), &actual))
	require.NoError(t, json.Unmarshal([]byte(body), &expected))
	for _, key := range []string{"system", "messages", "tools", "output_config", "anthropic_beta", "future_field"} {
		require.JSONEq(t, string(expected[key]), string(actual[key]), key)
	}
	require.NotContains(t, actual, "model")
	require.NotContains(t, actual, "stream")
	require.JSONEq(t, `"bedrock-2023-05-31"`, string(actual["anthropic_version"]))
	require.Contains(t, h.recorder.Body.String(), `"future_field":{"id":9007199254740993}`)
}

// TestClaudeInvokeExplicitARNWinsWithoutProbe checks override precedence and invocation count.
func TestClaudeInvokeExplicitARNWinsWithoutProbe(t *testing.T) {
	t.Parallel()
	h := newClaudeInvokeHarness(t, false, []byte(claudeInvokeReceipt), "us-east-1")
	h.meta.ActualModelName = "claude-sonnet-5"
	const target = "arn:aws:bedrock:us-east-1:123456789012:application-inference-profile/test-profile"
	mapping := `{"claude-sonnet-5":"` + target + `"}`
	h.context.Set(ctxkey.ChannelModel, &channelmodel.Channel{InferenceProfileArnMap: &mapping})
	h.prepareChat(t)
	_, responseErr := h.adaptor.DoResponse(h.context, nil, h.meta)
	require.Nil(t, responseErr)
	require.Equal(t, []string{"/model/" + target + "/invoke"}, h.transport.paths)
}

// TestClaudeInvokeStreamReceiptMatchesNonStream rejects addition of cumulative snapshots.
func TestClaudeInvokeStreamReceiptMatchesNonStream(t *testing.T) {
	t.Parallel()
	plain := newClaudeInvokeHarness(t, false, []byte(claudeInvokeReceipt), "test-region-1")
	plain.prepareChat(t)
	want, responseErr := plain.adaptor.DoResponse(plain.context, nil, plain.meta)
	require.Nil(t, responseErr)
	frames := claudeEventFrames(t, claudeInvokeStart, claudeInvokeTextStart, claudeInvokeText, claudeInvokeTextStop,
		`{"type":"message_delta","delta":{},"usage":{"output_tokens":3}}`, claudeInvokeDelta, claudeInvokeDelta, claudeInvokeStop)
	stream := newClaudeInvokeHarness(t, true, frames, "test-region-1")
	stream.prepareChat(t)
	got, responseErr := stream.adaptor.DoResponse(stream.context, nil, stream.meta)
	require.Nil(t, responseErr)
	require.Equal(t, want, got)
	require.Equal(t, 21, got.PromptTokens)
	require.Equal(t, 50, got.CompletionTokens)
	require.Equal(t, 1000, got.PromptTokensDetails.CachedTokens)
	require.Equal(t, 100, got.CacheWrite5mTokens)
	require.Equal(t, 200, got.CacheWrite1hTokens)
	require.Equal(t, 1, strings.Count(stream.recorder.Body.String(), "data: [DONE]"))
}

// TestClaudeInvokeNativeStreamKeepsClaudeProtocol checks named events and receipt accounting.
func TestClaudeInvokeNativeStreamKeepsClaudeProtocol(t *testing.T) {
	t.Parallel()
	h := newClaudeInvokeHarness(t, true, claudeEventFrames(t, claudeInvokeStart, claudeInvokeTextStart,
		claudeInvokeText, claudeInvokeTextStop, claudeInvokeDelta, claudeInvokeStop), "test-region-1")
	h.prepareNative(t, `{"model":"claude-3-haiku-20240307","stream":true,"max_tokens":256,"messages":[{"role":"user","content":"Hello"}]}`)
	usage, responseErr := h.adaptor.DoResponse(h.context, nil, h.meta)
	require.Nil(t, responseErr)
	require.Equal(t, 50, usage.CompletionTokens)
	require.Contains(t, h.recorder.Body.String(), "event: message_start\n")
	require.Contains(t, h.recorder.Body.String(), "event: message_stop\n")
	require.NotContains(t, h.recorder.Body.String(), "chat.completion.chunk")
	require.NotContains(t, h.recorder.Body.String(), "[DONE]")
}

// TestClaudeInvokeStreamFailuresDoNotComplete verifies SDK and protocol errors preserve partial usage.
func TestClaudeInvokeStreamFailuresDoNotComplete(t *testing.T) {
	t.Parallel()
	start := claudeEventFrames(t, claudeInvokeStart, claudeInvokeTextStart, claudeInvokeText)
	cases := map[string][]byte{
		"empty":           nil,
		"premature_eof":   start,
		"sdk_exception":   append(bytes.Clone(start), claudeExceptionFrame(t)...),
		"malformed_json":  append(bytes.Clone(start), claudeEventFrames(t, `{`)...),
		"anthropic_error": append(bytes.Clone(start), claudeEventFrames(t, `{"type":"error","error":{"type":"overloaded_error","message":"busy"}}`)...),
		"bad_crc":         append(bytes.Clone(start), []byte{0, 0, 0, 16, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}...),
	}
	for name, frames := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			h := newClaudeInvokeHarness(t, true, frames, "test-region-1")
			h.prepareChat(t)
			usage, responseErr := h.adaptor.DoResponse(h.context, nil, h.meta)
			require.NotNil(t, responseErr)
			require.NotContains(t, h.recorder.Body.String(), "[DONE]")
			if name != "empty" {
				require.NotNil(t, usage, "received usage must survive stream failure")
				require.Equal(t, 21, usage.PromptTokens)
				require.Equal(t, 1000, usage.PromptTokensDetails.CachedTokens)
			}
		})
	}
}
