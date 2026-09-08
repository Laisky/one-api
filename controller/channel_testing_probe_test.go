package controller

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/channeltype"
)

// upstreamRecorder is a fake provider that answers each chat surface in that
// surface's own wire shape and 404s everything else, while recording exactly
// which paths and bodies the probe sent.
//
// Answering in the correct per-path shape matters: a fake that returns the same
// JSON for every path lets a probe "succeed" while talking to the wrong endpoint,
// which is precisely the failure this suite has to be able to see.
type upstreamRecorder struct {
	server *httptest.Server
	mu     sync.Mutex
	paths  []string
	bodies []string
	status int
}

// newUpstreamRecorder starts a fake upstream.
// Parameters: t is the test handle and status is the HTTP status to answer with (200 for the happy path).
// Returns: the recorder, closed automatically when the test ends.
func newUpstreamRecorder(t *testing.T, status int) *upstreamRecorder {
	t.Helper()

	rec := &upstreamRecorder{status: status}
	rec.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		rec.mu.Lock()
		rec.paths = append(rec.paths, r.URL.Path)
		rec.bodies = append(rec.bodies, string(raw))
		rec.mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		if rec.status != http.StatusOK {
			w.WriteHeader(rec.status)
			_, _ = w.Write([]byte(`{"error":{"message":"upstream is down","type":"server_error"}}`))
			return
		}
		switch {
		case strings.HasSuffix(r.URL.Path, "/messages"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"id":"msg_1","type":"message","role":"assistant","model":"m",` +
				`"content":[{"type":"text","text":"PROBE_OK"}],"stop_reason":"end_turn",` +
				`"usage":{"input_tokens":7,"output_tokens":1}}`))
		case strings.HasSuffix(r.URL.Path, "/chat/completions"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"id":"c1","object":"chat.completion","model":"m",` +
				`"choices":[{"index":0,"message":{"role":"assistant","content":"PROBE_OK"},"finish_reason":"stop"}],` +
				`"usage":{"prompt_tokens":7,"completion_tokens":1,"total_tokens":8}}`))
		case strings.Contains(r.URL.Path, ":generateContent"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"candidates":[{"content":{"parts":[{"text":"PROBE_OK"}],"role":"model"},` +
				`"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":7,` +
				`"candidatesTokenCount":1,"totalTokenCount":8}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":{"message":"no such endpoint","type":"invalid_request_error"}}`))
		}
	}))
	t.Cleanup(rec.server.Close)
	return rec
}

// requestedPaths returns the upstream paths the probe hit.
// Returns: a copy of the recorded path list.
func (r *upstreamRecorder) requestedPaths() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string{}, r.paths...)
}

// requestedBodies returns the upstream request bodies the probe sent.
// Returns: a copy of the recorded body list.
func (r *upstreamRecorder) requestedBodies() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string{}, r.bodies...)
}

// testLogContents returns the content of every recorded test log row.
// Parameters: t is the test handle.
// Returns: the Content column of each log row, in insertion order.
func testLogContents(t *testing.T) []string {
	t.Helper()

	var logs []model.Log
	require.NoError(t, model.DB.Order("id asc").Find(&logs).Error)
	contents := make([]string, 0, len(logs))
	for _, entry := range logs {
		contents = append(contents, entry.Content)
	}
	return contents
}

// TestProbeUsesDeclaredChatSurface verifies the probe reaches the upstream endpoint
// and wire shape appropriate to each channel type.
//
// The probe is always built as one internal chat request; the adaptor is what
// translates it. This test is the evidence for that claim -- it asserts on the
// bytes that actually leave the gateway, not on the code path taken.
// Parameters: t is the test handle.
// Returns: no values.
func TestProbeUsesDeclaredChatSurface(t *testing.T) {
	cases := []struct {
		name          string
		channelType   int
		model         string
		config        string
		wantPath      string
		wantBodyHas   string
		wantBodyLacks string
	}{
		{
			name:          "openai-compatible channel probes the chat endpoint",
			channelType:   channeltype.OpenAICompatible,
			model:         "gpt-4o-mini",
			config:        "",
			wantPath:      "/v1/chat/completions",
			wantBodyHas:   `"messages"`,
			wantBodyLacks: `"max_tokens"`,
		},
		{
			// The adaptor, not the probe, decides the upstream surface: a
			// Claude-native channel is reached at /v1/messages with a Claude body
			// even though the probe is built as a chat request.
			name:          "claude-native channel is probed at the messages endpoint",
			channelType:   channeltype.Anthropic,
			model:         "claude-sonnet-4-20250514",
			config:        `{"supported_endpoints":["claude_messages"]}`,
			wantPath:      "/v1/messages",
			wantBodyHas:   `"max_tokens"`,
			wantBodyLacks: `"max_completion_tokens"`,
		},
		{
			name:          "claude-compatible channel is probed at the messages endpoint",
			channelType:   channeltype.ClaudeCompatible,
			model:         "claude-sonnet-4-20250514",
			config:        `{"supported_endpoints":["claude_messages"]}`,
			wantPath:      "/v1/messages",
			wantBodyHas:   `"max_tokens"`,
			wantBodyLacks: `"max_completion_tokens"`,
		},
		{
			// Gemini has no client-facing endpoint of its own: it is reached through
			// chat_completions and the adaptor rewrites the call to generateContent.
			name:          "gemini channel is probed at its native generateContent surface",
			channelType:   channeltype.Gemini,
			model:         "gemini-2.0-flash",
			config:        "",
			wantPath:      "/v1beta/models/gemini-2.0-flash:generateContent",
			wantBodyHas:   `"contents"`,
			wantBodyLacks: `"max_completion_tokens"`,
		},
		{
			name:          "gemini openai-compatible channel is probed at the chat endpoint",
			channelType:   channeltype.GeminiOpenAICompatible,
			model:         "gemini-2.0-flash",
			config:        "",
			wantPath:      "/chat/completions",
			wantBodyHas:   `"messages"`,
			wantBodyLacks: `"contents"`,
		},
		{
			name:          "response-api-only channel remains eligible and is probed",
			channelType:   channeltype.OpenAICompatible,
			model:         "gpt-4o-mini",
			config:        `{"supported_endpoints":["response_api"]}`,
			wantPath:      "/v1/chat/completions",
			wantBodyHas:   `"messages"`,
			wantBodyLacks: `"max_tokens"`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			setupChannelSweepTestEnvironment(t)
			upstream := newUpstreamRecorder(t, http.StatusOK)

			baseURL := upstream.server.URL
			channel := &model.Channel{
				Name:    "probe-target",
				Type:    tc.channelType,
				Status:  model.ChannelStatusEnabled,
				Key:     "sk-test",
				BaseURL: &baseURL,
				Models:  tc.model,
				Config:  tc.config,
			}
			require.NoError(t, model.DB.Create(channel).Error)

			runChannelSweep(t)

			require.Equal(t, []string{tc.wantPath}, upstream.requestedPaths(),
				"probe must reach the endpoint implied by the declared chat surface")
			bodies := upstream.requestedBodies()
			require.Len(t, bodies, 1)
			require.Contains(t, bodies[0], tc.wantBodyHas)
			require.NotContains(t, bodies[0], tc.wantBodyLacks)

			require.Equal(t, model.ChannelStatusEnabled, channelStatus(t, channel.Id),
				"a reachable channel must stay enabled")
			// Proving the response was actually understood, not merely 200: a probe
			// that cannot read the reply would log an empty response and still pass.
			require.Len(t, testLogContents(t), 1)
			require.Contains(t, testLogContents(t)[0], "PROBE_OK",
				"the probe must parse the upstream reply for this surface")
		})
	}
}

// TestClaudeOnlyChannelIsProbedNotSkipped pins the eligibility widening: before
// this change a channel narrowed to claude_messages had no "chat_completions"
// endpoint and was refused a probe entirely, which the sweep then treated as a
// failure. It must now be probed, and probed successfully.
// Parameters: t is the test handle.
// Returns: no values.
func TestClaudeOnlyChannelIsProbedNotSkipped(t *testing.T) {
	setupChannelSweepTestEnvironment(t)
	upstream := newUpstreamRecorder(t, http.StatusOK)

	baseURL := upstream.server.URL
	channel := &model.Channel{
		Name:    "claude-only",
		Type:    channeltype.Anthropic,
		Status:  model.ChannelStatusEnabled,
		Key:     "sk-test",
		BaseURL: &baseURL,
		Models:  "claude-sonnet-4-20250514",
		Config:  `{"supported_endpoints":["claude_messages"]}`,
	}
	require.NoError(t, model.DB.Create(channel).Error)

	runChannelSweep(t)

	require.Equal(t, []string{"/v1/messages"}, upstream.requestedPaths(),
		"a claude-only channel must actually be probed, not skipped")
	require.Equal(t, model.ChannelStatusEnabled, channelStatus(t, channel.Id))
	require.Len(t, testLogContents(t), 1)
	require.Contains(t, testLogContents(t)[0], "PROBE_OK")
}

// TestProbeReportsUnreachableSurfaceAsFailure verifies the probe still detects a
// genuine outage, so widening the skip path did not blunt the health check.
// Parameters: t is the test handle.
// Returns: no values.
func TestProbeReportsUnreachableSurfaceAsFailure(t *testing.T) {
	setupChannelSweepTestEnvironment(t)
	upstream := newUpstreamRecorder(t, http.StatusInternalServerError)

	baseURL := upstream.server.URL
	channel := &model.Channel{
		Name:    "broken-chat-channel",
		Type:    channeltype.OpenAICompatible,
		Status:  model.ChannelStatusEnabled,
		Key:     "sk-test",
		BaseURL: &baseURL,
		Models:  "gpt-4o-mini",
	}
	require.NoError(t, model.DB.Create(channel).Error)

	runChannelSweep(t)

	require.Equal(t, []string{"/v1/chat/completions"}, upstream.requestedPaths())
	require.Len(t, testLogContents(t), 1)
	require.Contains(t, testLogContents(t)[0], "failed",
		"an upstream 5xx must still be recorded as a failed probe")
	// A single 5xx deliberately does NOT auto-disable: monitor.ShouldDisableChannel
	// only disables on credential/quota/permission errors, never on one transient
	// server error. Pinning this keeps the skip work from being blamed for it.
	require.Equal(t, model.ChannelStatusEnabled, channelStatus(t, channel.Id))
}

// TestSweepNeverContactsUpstreamForSkippedChannel is the direct behavioural
// statement of "skipped": no request is made, no test log is written, and the
// channel's recorded latency is left untouched so the UI does not claim it was
// just tested.
// Parameters: t is the test handle.
// Returns: no values.
func TestSweepNeverContactsUpstreamForSkippedChannel(t *testing.T) {
	setupChannelSweepTestEnvironment(t)
	upstream := newUpstreamRecorder(t, http.StatusOK)

	baseURL := upstream.server.URL
	embeddingsOnly := &model.Channel{
		Name:    "embedding-only",
		Type:    channeltype.OpenAICompatible,
		Status:  model.ChannelStatusEnabled,
		Key:     "sk-test",
		BaseURL: &baseURL,
		Models:  "text-embedding-3-small,bge-m3",
	}
	translationOnly := &model.Channel{
		Name:    "deepl-translation-only",
		Type:    channeltype.DeepL,
		Status:  model.ChannelStatusEnabled,
		Key:     "sk-test",
		BaseURL: &baseURL,
		Models:  "deepl-en",
	}
	require.NoError(t, model.DB.Create(embeddingsOnly).Error)
	require.NoError(t, model.DB.Create(translationOnly).Error)

	runChannelSweep(t)

	require.Empty(t, upstream.requestedPaths(),
		"a skipped channel must never receive a probe request")
	require.Empty(t, testLogContents(t),
		"a skipped channel must not record a test log")

	for _, created := range []*model.Channel{embeddingsOnly, translationOnly} {
		var stored model.Channel
		require.NoError(t, model.DB.First(&stored, "id = ?", created.Id).Error)
		require.Equal(t, model.ChannelStatusEnabled, stored.Status, created.Name)
		require.Zero(t, stored.TestTime,
			"skipping must not stamp test_time on "+created.Name)
		require.Zero(t, stored.ResponseTime,
			"skipping must not stamp response_time on "+created.Name)
	}
}

// TestSweepSkipsUnknownFormatModelChannel is the end-to-end statement of the
// unknown-model policy: a self-hosted deployment whose name matches no catalogue
// entry is left alone by the sweep rather than probed on the assumption it is chat.
//
// Guessing chat is what sends a Chat Completions request to a private embeddings
// deployment; the request is meaningless there and its reply says nothing about the
// channel's health.
// Parameters: t is the test handle.
// Returns: no values.
func TestSweepSkipsUnknownFormatModelChannel(t *testing.T) {
	setupChannelSweepTestEnvironment(t)
	upstream := newUpstreamRecorder(t, http.StatusOK)

	baseURL := upstream.server.URL
	channel := &model.Channel{
		Name:    "self-hosted-unknown",
		Type:    channeltype.OpenAICompatible,
		Status:  model.ChannelStatusEnabled,
		Key:     "sk-test",
		BaseURL: &baseURL,
		Models:  "m3e-base,text2vec-large-chinese",
	}
	require.NoError(t, model.DB.Create(channel).Error)

	runChannelSweep(t)

	require.Empty(t, upstream.requestedPaths(),
		"a model of unknown API format must not be probed")
	require.Empty(t, testLogContents(t))
	require.Equal(t, model.ChannelStatusEnabled, channelStatus(t, channel.Id),
		"skipping must never disable the channel")
}

// TestSweepProbesUnknownModelWhenOperatorNamesIt verifies the override survives the
// whole sweep path: an administrator who sets testing_model opts that model back in.
// Parameters: t is the test handle.
// Returns: no values.
func TestSweepProbesUnknownModelWhenOperatorNamesIt(t *testing.T) {
	setupChannelSweepTestEnvironment(t)
	upstream := newUpstreamRecorder(t, http.StatusOK)

	baseURL := upstream.server.URL
	testingModel := "vendor/private-chat-model"
	channel := &model.Channel{
		Name:         "self-hosted-opted-in",
		Type:         channeltype.OpenAICompatible,
		Status:       model.ChannelStatusEnabled,
		Key:          "sk-test",
		BaseURL:      &baseURL,
		Models:       "vendor/private-chat-model",
		TestingModel: &testingModel,
	}
	require.NoError(t, model.DB.Create(channel).Error)

	runChannelSweep(t)

	require.Equal(t, []string{"/v1/chat/completions"}, upstream.requestedPaths(),
		"a deliberately named model must be probed even though its format is unknown")
	require.Len(t, testLogContents(t), 1)
	require.Contains(t, testLogContents(t)[0], "PROBE_OK")
	require.Equal(t, model.ChannelStatusEnabled, channelStatus(t, channel.Id))
}

// TestSweepSkipsChannelMarkedSkip verifies the per-channel SKIP opt-out: selecting
// SKIP as the channel's testing model excludes it from health checking entirely.
// Parameters: t is the test handle.
// Returns: no values.
func TestSweepSkipsChannelMarkedSkip(t *testing.T) {
	setupChannelSweepTestEnvironment(t)
	upstream := newUpstreamRecorder(t, http.StatusOK)

	baseURL := upstream.server.URL
	skip := model.ChannelTestingModelSkip
	channel := &model.Channel{
		Name:         "opted-out",
		Type:         channeltype.OpenAICompatible,
		Status:       model.ChannelStatusEnabled,
		Key:          "sk-test",
		BaseURL:      &baseURL,
		Models:       "gpt-4o-mini",
		TestingModel: &skip,
	}
	require.NoError(t, model.DB.Create(channel).Error)

	runChannelSweep(t)

	require.Empty(t, upstream.requestedPaths(),
		"a channel marked SKIP must never be probed even though it has a testable model")
	require.Empty(t, testLogContents(t))
	require.Equal(t, model.ChannelStatusEnabled, channelStatus(t, channel.Id))

	// The opt-out must survive the sweep: testing_model is not cleared as an
	// "invalid" model name would be.
	var stored model.Channel
	require.NoError(t, model.DB.First(&stored, "id = ?", channel.Id).Error)
	require.NotNil(t, stored.TestingModel)
	require.Equal(t, model.ChannelTestingModelSkip, *stored.TestingModel)
	require.Zero(t, stored.TestTime)
}

// TestSweepDisablesOnResponseTimeThreshold pins what ChannelDisableThreshold
// actually controls: it is a RESPONSE-TIME threshold in seconds, not a failure-rate
// percentage. The failure-rate mechanism is separate (MetricSuccessRateThreshold in
// monitor/metric.go). The admin tooltip documents this, so the behaviour is pinned.
// Parameters: t is the test handle.
// Returns: no values.
func TestSweepDisablesOnResponseTimeThreshold(t *testing.T) {
	setupChannelSweepTestEnvironment(t)

	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(150 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"c1","object":"chat.completion","model":"m",` +
			`"choices":[{"index":0,"message":{"role":"assistant","content":"PROBE_OK"},"finish_reason":"stop"}],` +
			`"usage":{"prompt_tokens":7,"completion_tokens":1,"total_tokens":8}}`))
	}))
	t.Cleanup(slow.Close)

	// 0.05s == 50ms, comfortably below the upstream's 150ms.
	config.ChannelDisableThreshold = 0.05

	baseURL := slow.URL
	channel := &model.Channel{
		Name:    "slow-but-healthy",
		Type:    channeltype.OpenAICompatible,
		Status:  model.ChannelStatusEnabled,
		Key:     "sk-test",
		BaseURL: &baseURL,
		Models:  "gpt-4o-mini",
	}
	require.NoError(t, model.DB.Create(channel).Error)

	runChannelSweep(t)

	require.Equal(t, model.ChannelStatusAutoDisabled, channelStatus(t, channel.Id),
		"a channel slower than the threshold is disabled even though the probe succeeded")
	require.Len(t, testLogContents(t), 1)
	require.Contains(t, testLogContents(t)[0], "succeed",
		"the probe itself passed; only the latency threshold disabled the channel")
}
