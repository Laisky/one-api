package controller

import (
	"bufio"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/model"
	metalib "github.com/Laisky/one-api/relay/meta"
	relaymodel "github.com/Laisky/one-api/relay/model"
)

// legacyWireRequest is one request the fake provider received.
type legacyWireRequest struct {
	Method, Path, RawQuery, Body string
	Header                       http.Header
}

// legacyWireReply is the fake provider's answer to one request. Truncate
// announces a longer body than Body and drops the connection after writing
// Body, so the relay reads Body and then a transport error.
type legacyWireReply struct {
	Status            int
	ContentType, Body string
	Truncate          bool
}

// legacyWireUpstream is a fake TLS provider that answers by request and
// records every request. Every production HTTP client (relay, impatient, and
// http.DefaultClient used by polling adaptors) dials it whatever host a URL
// names, so hard-coded provider hosts stay observable.
type legacyWireUpstream struct {
	server   *httptest.Server
	mu       sync.Mutex
	requests []legacyWireRequest
}

// newLegacyWireUpstream starts the fake provider. Parameters: t owns cleanup;
// reply maps a request to its answer. Returns: the started upstream.
func newLegacyWireUpstream(t *testing.T, reply func(r *http.Request) legacyWireReply) *legacyWireUpstream {
	t.Helper()
	upstream := &legacyWireUpstream{}
	upstream.server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		upstream.mu.Lock()
		upstream.requests = append(upstream.requests, legacyWireRequest{Method: r.Method, Path: r.URL.Path, RawQuery: r.URL.RawQuery, Body: string(body), Header: r.Header.Clone()})
		upstream.mu.Unlock()
		answer := reply(r)
		if answer.Status == 0 {
			answer.Status = http.StatusOK
		}
		w.Header().Set("Content-Type", answer.ContentType)
		if answer.Truncate {
			w.Header().Set("Content-Length", strconv.Itoa(len(answer.Body)+4096))
		}
		w.WriteHeader(answer.Status)
		_, _ = io.WriteString(w, answer.Body)
		if answer.Truncate {
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
			if hijacker, ok := w.(http.Hijacker); ok {
				if conn, _, err := hijacker.Hijack(); err == nil {
					_ = conn.Close()
				}
			}
		}
	}))
	t.Cleanup(upstream.server.Close)
	roots := x509.NewCertPool()
	roots.AddCert(upstream.server.Certificate())
	address := upstream.server.Listener.Addr().String()
	routed := &http.Client{Timeout: 10 * time.Second, Transport: &http.Transport{
		Proxy: nil,
		DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, network, address)
		},
		TLSClientConfig: &tls.Config{RootCAs: roots, ServerName: "example.com", MinVersion: tls.VersionTLS12},
	}}
	previousClient, previousImpatient, previousDefault := client.HTTPClient, client.ImpatientHTTPClient, http.DefaultClient
	client.HTTPClient, client.ImpatientHTTPClient, http.DefaultClient = routed, routed, routed
	t.Cleanup(func() {
		client.HTTPClient, client.ImpatientHTTPClient, http.DefaultClient = previousClient, previousImpatient, previousDefault
	})
	return upstream
}

// recorded returns a copy of the requests received so far.
func (u *legacyWireUpstream) recorded() []legacyWireRequest {
	u.mu.Lock()
	defer u.mu.Unlock()
	return append([]legacyWireRequest(nil), u.requests...)
}

// legacyClaudeFixture describes one legacy provider: its native wire request
// and its native stream and non-stream answers of the text "ack".
type legacyClaudeFixture struct {
	name    string
	channel int
	model   string
	key     string
	config  model.ChannelConfig
	// chatPath is the path (suffix) of the generation request; streamPath
	// overrides it for streaming when the provider streams from another path.
	chatPath, streamPath string
	// reply answers every provider request for the stream mode.
	reply func(stream bool, r *http.Request) legacyWireReply
	// wire checks the decoded generation request is the provider's native shape.
	wire func(t *testing.T, stream bool, body map[string]any)
	// receiptUsage reports whether the provider returns token usage that
	// settlement bills; otherwise usage is estimated by the adaptor.
	receiptUsage bool
	// reservationOnly reports a provider that returns no usage at all, so
	// settlement retains the admission reservation.
	reservationOnly bool
	// noChatStreamControl, when set, explains why the Chat Completions
	// controller cannot relay this provider's stream for the billing control.
	noChatStreamControl string
	// setup, when set, prepares provider credentials before each request.
	setup func(t *testing.T)
}

// legacyClaudeTariff prices every fixture model explicitly so admission and
// settlement are deterministic: 2 quota per input token, 6 per output token.
var legacyClaudeTariff = model.ModelConfigLocal{Ratio: 2, CompletionRatio: 3}

// legacyClaudeRequest returns the client endpoint and body for protocol.
// Parameters: protocol is chat or messages, modelName the client model, and
// stream selects SSE. Returns: the path and JSON body.
func legacyClaudeRequest(protocol, modelName string, stream bool) (string, string) {
	streamField := ""
	if stream {
		streamField = `,"stream":true`
	}
	body := `{"model":"` + modelName + `","messages":[{"role":"user","content":"Say ack"}],"max_tokens":64` + streamField + `}`
	if protocol == "messages" {
		return "/v1/messages", body
	}
	return "/v1/chat/completions", body
}

// legacyClaudeOutcome is what one relayed request produced.
type legacyClaudeOutcome struct {
	apiErr      *relaymodel.ErrorWithStatusCode
	writer      *httptest.ResponseRecorder
	generation  legacyWireRequest
	requests    []legacyWireRequest
	charge      int64
	consume     int64
	logQuota    int64
	costQuota   int64
	promptQuote int
}

// runLegacyClaudeFixture relays one request through the production helper
// for protocol against the fixture's fake provider and returns the outcome.
// Parameters: t owns assertions, fx the provider, protocol chat or messages,
// and stream selects SSE. Returns: the outcome with the ledger of the request.
func runLegacyClaudeFixture(t *testing.T, fx legacyClaudeFixture, protocol string, stream bool) legacyClaudeOutcome {
	t.Helper()
	const balance = int64(1_000_000)
	xaiVideoSetup(t, balance, false)
	if fx.setup != nil {
		fx.setup(t)
	}
	upstream := newLegacyWireUpstream(t, func(r *http.Request) legacyWireReply { return fx.reply(stream, r) })
	path, body := legacyClaudeRequest(protocol, fx.model, stream)
	tariff := legacyClaudeTariff
	c, writer, requestID := protocolContext(t, fx.channel, fx.model, path, body, upstream.server.URL, balance, 1, false, &tariff)
	c.Set(ctxkey.ModelMapping, map[string]string{})
	c.Set(ctxkey.RequestModel, fx.model)
	c.Set(ctxkey.Config, fx.config)
	c.Request.Header.Set("Authorization", "Bearer "+fx.key)
	clearNativeSettledLedger(t, requestID)

	var apiErr *relaymodel.ErrorWithStatusCode
	if protocol == "messages" {
		apiErr = RelayClaudeMessagesHelper(c)
	} else {
		apiErr = RelayTextHelper(c)
	}
	drainCriticalTasks(t)
	out := legacyClaudeOutcome{apiErr: apiErr, writer: writer, requests: upstream.recorded()}
	generationPath := fx.chatPath
	if stream && fx.streamPath != "" {
		generationPath = fx.streamPath
	}
	for _, request := range out.requests {
		if strings.HasSuffix(request.Path, generationPath) {
			out.generation = request
		}
	}
	out.charge = balance - reloadUserQuota(t)
	out.logQuota, out.costQuota = tariffRequestLedger(t, requestID)
	out.consume = consumeLogQuota(t, requestID)
	out.promptQuote = metalib.GetByContext(c).PromptTokens
	return out
}

// claudeSSEEvent is one parsed Claude Messages SSE event.
type claudeSSEEvent struct {
	Event string
	Data  map[string]any
}

// parseClaudeSSE parses a Claude Messages SSE body into events. Parameters: t
// owns assertions and body the raw SSE. Returns: the events in order.
func parseClaudeSSE(t *testing.T, body string) []claudeSSEEvent {
	t.Helper()
	var events []claudeSSEEvent
	var current claudeSSEEvent
	scanner := bufio.NewScanner(strings.NewReader(body))
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case strings.HasPrefix(line, "event:"):
			current.Event = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		case strings.HasPrefix(line, "data:"):
			payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			require.NotEqual(t, "[DONE]", payload, "a Claude stream never carries the chat [DONE] sentinel")
			require.NoError(t, json.Unmarshal([]byte(payload), &current.Data), "every Claude SSE data line is JSON: %q", payload)
			require.Equal(t, current.Event, current.Data["type"], "the SSE event name must equal the data type")
			events = append(events, current)
			current = claudeSSEEvent{}
		}
	}
	require.NoError(t, scanner.Err())
	return events
}

// requireClaudeMessageJSON asserts the client received exactly one Claude
// message whose only content is the text block "ack". Parameters: t owns
// assertions and writer the client recorder.
func requireClaudeMessageJSON(t *testing.T, writer *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	require.Contains(t, writer.Header().Get("Content-Type"), "application/json")
	var reply map[string]any
	require.NoError(t, json.Unmarshal(writer.Body.Bytes(), &reply), "the client must receive JSON: %s", writer.Body.String())
	require.Equal(t, "message", reply["type"], "a Claude Messages client must receive a Claude message; got %s", writer.Body.String())
	require.Equal(t, "assistant", reply["role"])
	require.Equal(t, []any{map[string]any{"type": "text", "text": "ack"}}, reply["content"])
	require.Equal(t, "end_turn", reply["stop_reason"])
	require.NotContains(t, reply, "choices", "no chat completion fields may leak to a Claude client")
	require.Contains(t, reply, "usage")
	return reply
}

// requireClaudeMessageSSE asserts the client received a complete, ordered
// Claude Messages stream whose text deltas spell "ack". Parameters: t owns
// assertions and writer the client recorder.
func requireClaudeMessageSSE(t *testing.T, writer *httptest.ResponseRecorder) []claudeSSEEvent {
	t.Helper()
	require.Contains(t, writer.Header().Get("Content-Type"), "text/event-stream")
	raw := writer.Body.String()
	require.NotContains(t, raw, "chat.completion.chunk", "no chat chunk may leak to a Claude client")
	events := parseClaudeSSE(t, raw)
	require.GreaterOrEqual(t, len(events), 5, "a Claude stream has start, block, delta, stop and terminal events; got %s", raw)
	require.Equal(t, "message_start", events[0].Event, "got %s", raw)
	require.Equal(t, "message_delta", events[len(events)-2].Event, "got %s", raw)
	require.Equal(t, "message_stop", events[len(events)-1].Event, "got %s", raw)
	delta, _ := events[len(events)-2].Data["delta"].(map[string]any)
	require.Equal(t, "end_turn", delta["stop_reason"])
	var text strings.Builder
	open := map[float64]bool{}
	for _, event := range events[1 : len(events)-2] {
		index, _ := event.Data["index"].(float64)
		switch event.Event {
		case "content_block_start":
			require.False(t, open[index], "a block starts once")
			open[index] = true
		case "content_block_delta":
			require.True(t, open[index], "deltas target an open block")
			if part, ok := event.Data["delta"].(map[string]any); ok && part["type"] == "text_delta" {
				text.WriteString(part["text"].(string))
			}
		case "content_block_stop":
			require.True(t, open[index], "only open blocks stop")
			open[index] = false
		default:
			require.Failf(t, "unexpected Claude event", "event %q in %s", event.Event, raw)
		}
	}
	for index, isOpen := range open {
		require.False(t, isOpen, "block %v must be stopped before message_delta", index)
	}
	require.Equal(t, "ack", text.String())
	return events
}

// TestLegacyAdaptorClaudeMessagesWireContract drives a Claude Messages request
// through RelayClaudeMessagesHelper for every legacy adaptor whose provider
// speaks a proprietary wire format and that advertises the Messages API by
// default. The fake provider answers in its native format; the provider must
// receive its native request, and the client must receive a Claude message
// (JSON) or a complete Claude Messages SSE stream, never OpenAI chat output.
// The same answer relayed to a Chat Completions client is the billing control:
// the Claude ledger must match it except for the documented admission pad.
func TestLegacyAdaptorClaudeMessagesWireContract(t *testing.T) {
	for _, fx := range legacyClaudeFixtures() {
		runLegacyClaudeContract(t, fx)
	}
}

// runLegacyClaudeContract runs the Claude Messages wire contract and its Chat
// Completions billing control for fx, non-stream and stream, as subtests.
// Parameters: t owns the subtests and fx is the provider fixture.
func runLegacyClaudeContract(t *testing.T, fx legacyClaudeFixture) {
	t.Helper()
	for _, stream := range []bool{false, true} {
		name := fx.name + "/json"
		if stream {
			name = fx.name + "/stream"
		}
		t.Run(name, func(t *testing.T) {
			claude := runLegacyClaudeFixture(t, fx, "messages", stream)
			require.Nil(t, claude.apiErr, "the Claude request must succeed: %+v; body %s", claude.apiErr, claude.writer.Body.String())
			require.NotEmpty(t, claude.generation.Path, "the provider must receive the generation request; got %+v", claude.requests)
			if stream {
				requireClaudeMessageSSE(t, claude.writer)
			} else {
				requireClaudeMessageJSON(t, claude.writer)
			}
			var wire map[string]any
			require.NoError(t, json.Unmarshal([]byte(claude.generation.Body), &wire), "the provider request must be JSON: %s", claude.generation.Body)
			fx.wire(t, stream, wire)
			require.Positive(t, claude.charge, "the Claude request must be charged")
			require.Equal(t, claude.charge, claude.consume, "the consume log must record the balance delta")
			require.Equal(t, claude.charge, claude.logQuota, "no provisional row may retain an extra charge")
			require.Equal(t, claude.charge, claude.costQuota, "the request cost must reconcile to the charge")

			if fx.receiptUsage {
				// 11 input tokens at 2 plus 2 output tokens at 2*3.
				require.EqualValues(t, 34, claude.charge, "the provider receipt is what settlement bills")
			}
			if stream && fx.noChatStreamControl != "" {
				t.Logf("no Chat Completions stream control: %s", fx.noChatStreamControl)
				return
			}

			chat := runLegacyClaudeFixture(t, fx, "chat", stream)
			require.Nil(t, chat.apiErr, "the Chat Completions control must succeed: %+v", chat.apiErr)
			require.Contains(t, chat.writer.Body.String(), `"choices"`, "control: Chat Completions clients keep chat output")
			require.Equal(t, chat.charge, chat.consume, "control: the consume log must record the balance delta")
			require.Equal(t, chat.promptQuote, claude.promptQuote, "both protocols must estimate the same prompt")
			if fx.reservationOnly {
				// Without a receipt both settle on their reservation; only Chat
				// admission pads the prompt with PreConsumedQuota.
				require.Equal(t, config.PreConsumedQuota*int64(legacyClaudeTariff.Ratio), chat.charge-claude.charge,
					"Claude and Chat clients must settle the same reservation apart from the Chat admission pad")
				return
			}
			require.Equal(t, chat.charge, claude.charge, "the same provider answer must bill identically for Chat and Claude clients")
		})
	}
}
