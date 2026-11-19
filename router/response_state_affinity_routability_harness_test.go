package router

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/channeltype"
	relaymodel "github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/state"
)

// These tests drive provider affinity for the gateway Response-State layer
// through the shipped public router: real token auth, middleware.Distribute
// (which resolves the affinity pin), the Responses controller with its Chat
// fallback, state hydration/commit and billing. They must NOT run in parallel:
// state.SetForTest and the routing configuration are process-global.

const (
	// affinityBoundChannelAssistant is the reply of the provider a response is
	// first bound to (channel A). It only reaches channel B through replay.
	affinityBoundChannelAssistant = "bound-provider-A-reply-7311"
	// affinityAlternateAssistant is the reply of the higher-priority provider
	// (channel B) that normal selection prefers.
	affinityAlternateAssistant = "alternate-provider-B-reply-5927"
	// affinityPriorUserText is the first turn's user input recorded under R.
	affinityPriorUserText = "remember the affinity marker 8842"
	// affinityAlternateChannelID is the fixture id of channel B.
	affinityAlternateChannelID = 929295
	// affinityOtherGroup is a second group channel A serves, used to prove a
	// cooldown in another group does not unpin the binding.
	affinityOtherGroup = "vip"
)

// affinityUpstream is a discriminating fake OpenAI-compatible chat-completions
// provider. It records every request body it receives and always answers with
// its own assistant text, so the client response and the call counters reveal
// which provider served a turn.
type affinityUpstream struct {
	mu        sync.Mutex
	bodies    [][]byte
	assistant string
	server    *httptest.Server
}

// newAffinityUpstream starts a fake provider answering with assistant and
// registers its shutdown on t. It returns the recording upstream.
func newAffinityUpstream(t *testing.T, assistant string) *affinityUpstream {
	t.Helper()
	up := &affinityUpstream{assistant: assistant}
	up.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read upstream request body: %v", err)
			http.Error(w, "read failed", http.StatusBadRequest)
			return
		}
		up.mu.Lock()
		up.bodies = append(up.bodies, body)
		up.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if _, err := fmt.Fprintf(w, `{"id":"chatcmpl-affinity","object":"chat.completion","created":1767225600,"model":"gpt-4o-mini","choices":[{"index":0,"message":{"role":"assistant","content":%q},"finish_reason":"stop"}],"usage":{"prompt_tokens":11,"completion_tokens":7,"total_tokens":18}}`, up.assistant); err != nil {
			t.Errorf("write upstream response: %v", err)
		}
	}))
	t.Cleanup(up.server.Close)
	return up
}

// calls returns how many requests the upstream has received so far.
func (u *affinityUpstream) calls() int {
	u.mu.Lock()
	defer u.mu.Unlock()
	return len(u.bodies)
}

// lastChatRequest decodes the most recent request body the upstream received.
// It fails the test when the upstream was never contacted or the body is not
// a Chat Completions request, and returns the decoded request.
func (u *affinityUpstream) lastChatRequest(t *testing.T) relaymodel.GeneralOpenAIRequest {
	t.Helper()
	u.mu.Lock()
	defer u.mu.Unlock()
	require.NotEmpty(t, u.bodies, "expected the upstream to be contacted")
	var req relaymodel.GeneralOpenAIRequest
	require.NoError(t, json.Unmarshal(u.bodies[len(u.bodies)-1], &req), "decode upstream chat request")
	return req
}

// affinityTurn is the client-observable outcome of one public /v1/responses call.
type affinityTurn struct {
	status int
	body   string
	id     string
	text   string
}

// postAffinityTurn sends one authenticated POST /v1/responses with payload to
// the public server, waits for detached billing, and returns the status, raw
// body, response id and concatenated assistant output text.
func postAffinityTurn(t *testing.T, server *httptest.Server, key, payload string) affinityTurn {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, server.URL+"/v1/responses", strings.NewReader(payload))
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer sk-"+key)
	req.Header.Set("Content-Type", "application/json")
	httpClient := server.Client()
	httpClient.Timeout = 10 * time.Second
	resp, err := httpClient.Do(req)
	require.NoError(t, err)
	raw, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	drainCriticalTasksForRouter(t)

	turn := affinityTurn{status: resp.StatusCode, body: string(raw)}
	if resp.StatusCode != http.StatusOK {
		return turn
	}
	var decoded struct {
		ID     string `json:"id"`
		Output []struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		} `json:"output"`
	}
	require.NoError(t, json.Unmarshal(raw, &decoded), "decode Responses body: %s", raw)
	turn.id = decoded.ID
	var text strings.Builder
	for _, item := range decoded.Output {
		for _, part := range item.Content {
			text.WriteString(part.Text)
		}
	}
	turn.text = text.String()
	return turn
}

// affinityHarness is the public router wired with channel A (the binding
// target), the optional higher-priority channel B and the Response-State layer.
type affinityHarness struct {
	server   *httptest.Server
	key      string
	upA      *affinityUpstream
	upB      *affinityUpstream
	channelA int
	memory   bool
}

// newAffinityHarness builds the shipped relay router on an isolated SQLite
// ledger with channel A serving passthroughKeysAllowedModel and
// passthroughKeysPremiumModel in the default and affinityOtherGroup groups,
// enables the gateway Response-State layer with an in-memory store, disables
// retries, and selects the memory-cache or database routing path by memory.
// It returns the harness; channel B is added later by addAlternateChannel.
func newAffinityHarness(t *testing.T, memory bool) *affinityHarness {
	t.Helper()
	upA := newAffinityUpstream(t, affinityBoundChannelAssistant)
	upB := newAffinityUpstream(t, affinityAlternateAssistant)
	models := passthroughKeysAllowedModel + "," + passthroughKeysPremiumModel
	engine, key := ambiguousKeysRouter(t, upA.server.URL, channeltype.OpenAICompatible, models, true)

	// Both fake providers are plain HTTP, so one transport reaches either.
	upstreamClient := upA.server.Client()
	upstreamClient.Timeout = 5 * time.Second
	client.HTTPClient = upstreamClient

	oldRetries, oldBatch, oldMemory := config.RetryTimes, config.BatchUpdateEnabled, config.MemoryCacheEnabled
	config.RetryTimes, config.BatchUpdateEnabled = 0, false
	t.Cleanup(func() {
		config.RetryTimes, config.BatchUpdateEnabled, config.MemoryCacheEnabled = oldRetries, oldBatch, oldMemory
	})

	state.SetForTest(state.NewMemoryStore(state.DefaultLimits()))
	t.Cleanup(func() { state.SetForTest(nil) })
	require.True(t, state.Enabled(), "the Response-State layer must be active for this scenario")

	// Channel A additionally serves another group so a cooldown scoped to that
	// group has an exact ability row to suspend.
	var channelA model.Channel
	require.NoError(t, model.DB.Take(&channelA).Error)
	require.NoError(t, model.DB.Model(&model.Channel{}).Where("id = ?", channelA.Id).
		Update("group", "default,"+affinityOtherGroup).Error)
	require.NoError(t, model.DB.Where("id = ?", channelA.Id).Take(&channelA).Error)
	require.NoError(t, channelA.DeleteAbilities())
	require.NoError(t, channelA.AddAbilities())

	h := &affinityHarness{key: key, upA: upA, upB: upB, channelA: channelA.Id, memory: memory}
	config.MemoryCacheEnabled = memory
	h.refreshRoutingCache(t)
	h.server = httptest.NewServer(engine)
	t.Cleanup(h.server.Close)
	return h
}

// refreshRoutingCache rebuilds the in-memory channel cache after a channel
// change when the memory-cache routing path is selected; it is a no-op on the
// database path.
func (h *affinityHarness) refreshRoutingCache(t *testing.T) {
	t.Helper()
	if h.memory {
		require.NoError(t, model.InitChannelCacheContext(context.Background()))
	}
}

// addAlternateChannel adds channel B for the same group and model with a
// HIGHER priority than channel A, so normal selection prefers B and any turn
// served by A is observably the affinity pin.
func (h *affinityHarness) addAlternateChannel(t *testing.T) {
	t.Helper()
	priority := int64(10)
	base := h.upB.server.URL
	ch := &model.Channel{
		Id: affinityAlternateChannelID, UUID: uuid.NewString(), Type: channeltype.OpenAICompatible,
		Status: model.ChannelStatusEnabled, Name: "affinity-alternate", Models: passthroughKeysAllowedModel,
		Group: "default", BaseURL: &base, Key: "fixture-only", Priority: &priority,
	}
	require.NoError(t, model.DB.Create(ch).Error)
	require.NoError(t, ch.AddAbilities())
	h.refreshRoutingCache(t)
}

// turn posts payload and returns the outcome together with how many calls
// channel A and channel B received for that single turn.
func (h *affinityHarness) turn(t *testing.T, payload string) (affinityTurn, int, int) {
	t.Helper()
	beforeA, beforeB := h.upA.calls(), h.upB.calls()
	got := postAffinityTurn(t, h.server, h.key, payload)
	return got, h.upA.calls() - beforeA, h.upB.calls() - beforeB
}

// continuation returns a Responses payload continuing previousID with input.
func continuation(previousID, input string) string {
	return `{"model":"` + passthroughKeysAllowedModel + `","stream":false,"previous_response_id":"` + previousID + `","input":"` + input + `"}`
}

// requireReplayedPriorTurn asserts that a Chat Completions request carries the
// canonical replay of the first turn (its user input and channel A's assistant
// reply) ahead of the new user input followUp.
func requireReplayedPriorTurn(t *testing.T, req relaymodel.GeneralOpenAIRequest, followUp string) {
	t.Helper()
	priorUser, priorAssistant, newUser := -1, -1, -1
	for i, msg := range req.Messages {
		content := msg.StringContent()
		switch {
		case msg.Role == "user" && strings.Contains(content, affinityPriorUserText):
			priorUser = i
		case msg.Role == "assistant" && strings.Contains(content, affinityBoundChannelAssistant):
			priorAssistant = i
		case msg.Role == "user" && strings.Contains(content, followUp):
			newUser = i
		}
	}
	require.GreaterOrEqual(t, priorUser, 0, "replay must carry the prior user input; messages=%+v", req.Messages)
	require.Greater(t, priorAssistant, priorUser, "replay must carry the prior assistant output after its input; messages=%+v", req.Messages)
	require.Greater(t, newUser, priorAssistant, "the new input must follow the replayed turn; messages=%+v", req.Messages)
}

// TestResponseStateAffinityCooldownPublicRouter is the end-to-end regression
// for PR #627's affinity cooldown: a Responses continuation bound to channel A
// stays pinned to A while A is healthy (even though higher-priority channel B
// exists), stays pinned when A cools down only for another model or group, and
// falls back to B — with the prior turn replayed canonically — once A's exact
// (group, model, channel) ability is cooling down. Both the database and the
// memory-cache routing paths are covered.
func TestResponseStateAffinityCooldownPublicRouter(t *testing.T) {
	for _, memory := range []bool{false, true} {
		t.Run(fmt.Sprintf("cache=%t", memory), func(t *testing.T) {
			h := newAffinityHarness(t, memory)
			ctx := context.Background()

			// Turn 1: only channel A exists; the stored response R is bound to A.
			first, callsA, callsB := h.turn(t, `{"model":"`+passthroughKeysAllowedModel+`","stream":false,"store":true,"input":"`+affinityPriorUserText+`"}`)
			require.Equal(t, http.StatusOK, first.status, first.body)
			require.Equal(t, 1, callsA, "turn 1 must be served by channel A")
			require.Zero(t, callsB)
			require.Equal(t, affinityBoundChannelAssistant, first.text, first.body)
			require.True(t, state.LooksLikeGatewayResponseID(first.id), "turn 1 must return a gateway response id, got %q", first.id)

			h.addAlternateChannel(t)

			// Baseline: without a state selector normal selection prefers channel B,
			// so a continuation served by A below can only be the affinity pin.
			baseline, callsA, callsB := h.turn(t, `{"model":"`+passthroughKeysAllowedModel+`","stream":false,"store":false,"input":"stateless baseline"}`)
			require.Equal(t, http.StatusOK, baseline.status, baseline.body)
			require.Zero(t, callsA, "normal selection must not pick lower-priority channel A")
			require.Equal(t, 1, callsB, "normal selection must pick higher-priority channel B")
			require.Equal(t, affinityAlternateAssistant, baseline.text, baseline.body)

			// Control: no cooldown, so the continuation of R is pinned to A.
			control, callsA, callsB := h.turn(t, continuation(first.id, "control follow-up"))
			require.Equal(t, http.StatusOK, control.status, control.body)
			require.Equal(t, 1, callsA, "a healthy binding must pin the continuation to channel A")
			require.Zero(t, callsB, "a pinned continuation must not reach channel B")
			require.Equal(t, affinityBoundChannelAssistant, control.text, control.body)

			// Scope: cooldowns of A's abilities for another model or another group
			// do not cover (default, model, A), so the binding still pins A.
			require.NoError(t, model.SuspendAbility(ctx, "default", passthroughKeysPremiumModel, h.channelA, time.Hour))
			require.NoError(t, model.SuspendAbility(ctx, affinityOtherGroup, passthroughKeysAllowedModel, h.channelA, time.Hour))
			scoped, callsA, callsB := h.turn(t, continuation(first.id, "scoped follow-up"))
			require.Equal(t, http.StatusOK, scoped.status, scoped.body)
			require.Equal(t, 1, callsA, "a cooldown for another model or group must not unpin channel A")
			require.Zero(t, callsB)
			require.Equal(t, affinityBoundChannelAssistant, scoped.text, scoped.body)

			// Cooldown: A's exact (default, model, A) ability is suspended, so the
			// continuation is unpinned, served by B, and replays R canonically.
			require.NoError(t, model.SuspendAbility(ctx, "default", passthroughKeysAllowedModel, h.channelA, time.Hour))
			const followUp = "cooldown follow-up 3307"
			cooled, callsA, callsB := h.turn(t, continuation(first.id, followUp))
			require.Equal(t, http.StatusOK, cooled.status, cooled.body)
			require.Zero(t, callsA, "a continuation must not reach channel A while its exact ability is cooling down")
			require.Equal(t, 1, callsB, "the unpinned continuation must be served by channel B")
			require.Equal(t, affinityAlternateAssistant, cooled.text, cooled.body)
			requireReplayedPriorTurn(t, h.upB.lastChatRequest(t), followUp)
		})
	}
}
