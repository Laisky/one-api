package gemini

import (
	"encoding/base64"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/realtime"
)

// testLiveGate funds work in token units: every estimated or receipted token
// costs one unit. A negative limit funds everything. It records each decision.
type testLiveGate struct {
	mu        sync.Mutex
	limit     int64
	committed int64
	ensured   []realtime.Estimate
	refused   int
	evidence  []realtime.Estimate
}

// newTestLiveGate returns a gate with limit units. Parameters: limit is the
// budget or negative for unlimited. Returns: the gate.
func newTestLiveGate(limit int64) *testLiveGate { return &testLiveGate{limit: limit} }

// Commit adds receipt tokens. Parameters: record is a receipt. Returns: none.
func (g *testLiveGate) Commit(record realtime.Record) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.committed += record.Tokens.Input + record.Tokens.Output
}

// Ensure funds committed plus pending tokens within the limit. Parameters:
// pending is the exposure. Returns: an exhausted error beyond the limit.
func (g *testLiveGate) Ensure(pending realtime.Estimate) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.ensured = append(g.ensured, pending)
	if g.limit >= 0 && g.committed+pending.TotalTokens() > g.limit {
		g.refused++
		return errors.WithStack(realtime.ErrBudgetExhausted)
	}
	return nil
}

// Finish records evidence. Parameters: evidence is unreceipted work. Returns: none.
func (g *testLiveGate) Finish(evidence realtime.Estimate) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.evidence = append(g.evidence, evidence)
}

// snapshot returns the recorded decisions. Parameters: none. Returns: copies.
func (g *testLiveGate) snapshot() ([]realtime.Estimate, int, []realtime.Estimate) {
	g.mu.Lock()
	defer g.mu.Unlock()
	return append([]realtime.Estimate(nil), g.ensured...), g.refused, append([]realtime.Estimate(nil), g.evidence...)
}

// TestLiveSpendTurnExposure verifies the per-turn funding model: context is
// re-billed by each turn, streamed output and allowances are funded, receipts
// replace estimates and evidence excludes allowances of unstarted turns.
// Parameters: t owns assertions. Returns: none.
func TestLiveSpendTurnExposure(t *testing.T) {
	t.Parallel()
	gate := newTestLiveGate(-1)
	spend := &liveSpend{gate: gate, allowance: 100, carried: 40}
	require.NoError(t, spend.admit(realtime.Estimate{Text: 10}))
	require.NoError(t, spend.admit(realtime.Estimate{Audio: 5}))
	ensured, _, _ := gate.snapshot()
	require.Equal(t, realtime.Estimate{Text: 10, Audio: 5, Context: 40, Output: 100}, ensured[len(ensured)-1],
		"a pending turn re-bills the setup context plus a per-turn output allowance")

	audio := []byte(`{"serverContent":{"modelTurn":{"parts":[{"inlineData":{"mimeType":"audio/pcm;rate=24000","data":"` +
		strings.Repeat("A", 64000) + `"}}]}}}`)
	require.NoError(t, spend.observe(audio, true, nil))
	ensured, _, _ = gate.snapshot()
	streamed := pcmTokens(base64DecodedBound(strings.Repeat("A", 64000)), liveOutputSampleRate)
	require.Equal(t, realtime.Estimate{Text: 10, Audio: 5, Context: 40, Output: 100, OutputAudio: streamed}, ensured[len(ensured)-1],
		"the in-flight turn keeps its inputs, streamed output and allowance")

	require.NoError(t, spend.admit(realtime.Estimate{Text: 7}))
	ensured, _, _ = gate.snapshot()
	inflight := realtime.Estimate{Text: 10, Audio: 5, Context: 40, Output: 100, OutputAudio: streamed}
	require.Equal(t, inflight.Add(realtime.Estimate{Text: 7, Context: inflight.TotalTokens(), Output: 100}), ensured[len(ensured)-1],
		"input during a turn funds a next turn whose context contains the whole in-flight turn")

	receipt := realtime.Record{Tokens: realtime.Tokens{Input: 60, Text: 60, Output: 20, OutputAudio: 20}}
	require.NoError(t, spend.observe([]byte(`{"serverContent":{"turnComplete":true}}`), true, []realtime.Record{receipt}))
	ensured, _, _ = gate.snapshot()
	require.Equal(t, realtime.Estimate{Text: 7, Context: 80, Output: 100}, ensured[len(ensured)-1],
		"the receipt replaces estimates and its prompt plus output become the carried context")
	require.EqualValues(t, 80, gate.committed)

	spend.finish()
	_, _, evidence := gate.snapshot()
	require.Equal(t, []realtime.Estimate{{Text: 7}}, evidence, "evidence excludes allowances and unstarted context")
}

// TestLiveSpendRefusalIsFinal verifies a refused operation leaves the tracker
// unchanged and that the session can never resume afterwards. Parameters: t
// owns assertions. Returns: none.
func TestLiveSpendRefusalIsFinal(t *testing.T) {
	t.Parallel()
	gate := newTestLiveGate(150)
	spend := &liveSpend{gate: gate, allowance: 100, carried: 40}
	require.NoError(t, spend.admit(realtime.Estimate{Text: 10}))
	require.ErrorIs(t, spend.admit(realtime.Estimate{Text: 1}), realtime.ErrBudgetExhausted)
	require.Equal(t, realtime.Estimate{Text: 10}, spend.pending, "a refused frame is not counted as forwarded")
	require.ErrorIs(t, spend.admit(realtime.Estimate{}), realtime.ErrBudgetExhausted)
	require.ErrorIs(t, spend.observe([]byte(`{}`), false, nil), realtime.ErrBudgetExhausted)
	require.True(t, spend.isExhausted())
	_, refused, _ := gate.snapshot()
	require.Equal(t, 1, refused, "later calls fail locally without asking the gate again")
}

// liveSpendSetupTokens returns the tracker's initial context for the fixture
// setup. Parameters: t owns assertions. Returns: tokens.
func liveSpendSetupTokens(t *testing.T) int64 {
	t.Helper()
	setup, err := prepareLiveSetup([]byte(`{"setup":{"model":"friendly","inputAudioTranscription":{},"outputAudioTranscription":{},"tools":[{"functionDeclarations":[{"name":"lookup","parameters":{"type":"OBJECT"}}]}]}}`),
		"gemini-3.8-live", "friendly")
	require.NoError(t, err)
	spend, err := newLiveSpend(newTestLiveGate(-1), setup, nil)
	require.NoError(t, err)
	require.EqualValues(t, liveDefaultTurnOutputTokens, spend.allowance)
	return spend.carried
}

// liveCountingProvider counts forwarded client frames. Parameters: frames
// receives the count. Returns: a provider script.
func liveCountingProvider(frames chan<- []string) func(*websocket.Conn) error {
	return func(up *websocket.Conn) error {
		if err := acknowledgeLiveFixture(up, "gemini-3.8-live"); err != nil {
			return err
		}
		var seen []string
		defer func() { frames <- seen }()
		for {
			_, raw, err := up.ReadMessage()
			if err != nil {
				return nil
			}
			seen = append(seen, string(raw))
		}
	}
}

// liveClientCloseReason reads until the gateway closes. Parameters: conn is the
// client socket. Returns: the close reason or "" on timeout.
func liveClientCloseReason(conn *websocket.Conn) string {
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			var closeErr *websocket.CloseError
			if errors.As(err, &closeErr) {
				return closeErr.Text
			}
			return ""
		}
	}
}

// TestLiveSpendGatesEveryClientOperation proves on real sockets that text,
// activity controls and unpriceable inputs pass the funding gate before any
// byte reaches the provider. Parameters: t owns the test. Returns: none.
func TestLiveSpendGatesEveryClientOperation(t *testing.T) {
	setupTokens := liveSpendSetupTokens(t)
	headroom := setupTokens + liveDefaultTurnOutputTokens
	text := `{"realtimeInput":{"text":"` + strings.Repeat("x", 1000) + `"}}`
	pcm := base64.StdEncoding.EncodeToString(make([]byte, 32000)) // 1 s at 16 kHz
	audio := `{"realtimeInput":{"audio":{"mimeType":"audio/pcm;rate=16000","data":"` + pcm + `"}}}`
	audioTokens := pcmTokens(base64DecodedBound(pcm), liveDefaultInputSampleRate)
	video := `{"realtimeInput":{"video":{"mimeType":"image/png","data":"` + liveTestImage(t, 10, 10, "png") + `"}}}`
	for _, tc := range []struct {
		name, frame string
		limit       int64
		sent, want  int
		reason      string
		gap         bool
	}{
		{"text_burst", text, headroom + 3000, 10, 3, liveCloseQuotaExhausted, true},
		{"audio_burst_faster_than_real_time", audio, headroom + 3*audioTokens, 10, 3, liveCloseQuotaExhausted, true},
		{"visual_burst", video, headroom + 2*liveImageTokenFloor, 10, 2, liveCloseQuotaExhausted, true},
		{"controls_are_free_but_gated", `{"realtimeInput":{"activityEnd":{}}}`, headroom, 50, 50, "", false},
		{"controls_cannot_start_unfunded_turns", `{"realtimeInput":{"activityEnd":{}}}`, headroom - 1, 5, 0, liveCloseQuotaExhausted, false},
		{"file_reference_fails_closed", `{"clientContent":{"turns":[{"role":"user","parts":[{"fileData":{"fileUri":"https://example.invalid/f","mimeType":"video/mp4"}}]}],"turnComplete":true}}`, -1, 1, 0, liveCloseUnpriceableWork, false},
		{"unknown_audio_codec_fails_closed", `{"realtimeInput":{"audio":{"mimeType":"audio/ogg","data":"AAAA"}}}`, -1, 1, 0, liveCloseUnpriceableWork, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			frames := make(chan []string, 1)
			gate := newTestLiveGate(tc.limit)
			endpoint, results := liveFixtureWithGate(t, channeltype.Gemini, "gemini-3.8-live", gate, liveCountingProvider(frames))
			client := connectLiveFixture(t, endpoint)
			for range tc.sent {
				if err := client.WriteMessage(websocket.TextMessage, []byte(tc.frame)); err != nil {
					break
				}
			}
			reason := ""
			if tc.reason != "" {
				reason = liveClientCloseReason(client)
			} else {
				time.Sleep(200 * time.Millisecond)
				_ = client.Close()
			}
			usage := receiveLiveFixture(t, results)
			seen := <-frames
			require.Len(t, seen, tc.want, "only funded operations reach the provider")
			for _, frame := range seen {
				require.Equal(t, tc.frame, frame, "funding never rewrites native frames")
			}
			require.Equal(t, tc.reason, reason)
			_, _, evidence := gate.snapshot()
			require.Len(t, evidence, 1, "the tracker reports evidence exactly once")
			require.Equal(t, tc.gap, usage.Realtime.HasUsageGap(), "forwarded unreceipted input stays an explicit gap")
		})
	}
}

// TestLiveSpendGatesFunctionResponses verifies a genuine function result is
// funded like any other input: a small result is forwarded, an unfunded large
// one is not. Parameters: t owns the test. Returns: none.
func TestLiveSpendGatesFunctionResponses(t *testing.T) {
	small := `{"toolResponse":{"functionResponses":[{"id":"c1","name":"lookup","response":{"ok":true}}]}}`
	large := `{"toolResponse":{"functionResponses":[{"id":"c2","name":"lookup","response":{"text":"` + strings.Repeat("y", 4000) + `"}}]}}`
	call := func(id string) []byte {
		return []byte(`{"toolCall":{"functionCalls":[{"id":"` + id + `","name":"lookup","args":{}}]}}`)
	}
	// Dry-run the funding sequence: the call starts a turn and the small result
	// funds the next one. The large result adds 4 KB the budget does not have.
	dry := &liveSpend{gate: newTestLiveGate(-1), allowance: liveDefaultTurnOutputTokens, carried: liveSpendSetupTokens(t)}
	require.NoError(t, dry.observe(call("c1"), true, nil))
	smallInput, err := estimateLiveClientFrame([]byte(small))
	require.NoError(t, err)
	require.NoError(t, dry.admit(smallInput))
	ensured, _, _ := dry.gate.(*testLiveGate).snapshot()
	gate := newTestLiveGate(ensured[len(ensured)-1].TotalTokens() + 1000)
	frames := make(chan []string, 1)
	endpoint, results := liveFixtureWithGate(t, channeltype.Gemini, "gemini-3.8-live", gate, func(up *websocket.Conn) error {
		if err := acknowledgeLiveFixture(up, "gemini-3.8-live"); err != nil {
			return err
		}
		var seen []string
		defer func() { frames <- seen }()
		for _, id := range []string{"c1", "c2"} {
			if err := liveWrite(up, websocket.TextMessage, call(id), time.Second); err != nil {
				return errors.Wrap(err, "write function call")
			}
			_, raw, err := up.ReadMessage()
			if err != nil {
				return nil
			}
			seen = append(seen, string(raw))
		}
		return nil
	})
	client := connectLiveFixture(t, endpoint)
	for _, result := range []string{small, large} {
		if _, _, err := client.ReadMessage(); err != nil {
			break
		}
		if err := client.WriteMessage(websocket.TextMessage, []byte(result)); err != nil {
			break
		}
	}
	require.Equal(t, liveCloseQuotaExhausted, liveClientCloseReason(client))
	receiveLiveFixture(t, results)
	require.Equal(t, []string{small}, <-frames)
}

// TestLiveSpendStopsUnfundedProviderOutput verifies streamed output beyond the
// budget is withheld and the provider session is closed before its receipt.
// Parameters: t owns the test. Returns: none.
func TestLiveSpendStopsUnfundedProviderOutput(t *testing.T) {
	chunk := []byte(`{"serverContent":{"modelTurn":{"parts":[{"inlineData":{"mimeType":"audio/pcm;rate=24000","data":"` +
		strings.Repeat("A", 64000) + `"}}]}}}`)
	perChunk := pcmTokens(base64DecodedBound(strings.Repeat("A", 64000)), liveOutputSampleRate)
	text := `{"realtimeInput":{"text":"hi"}}`
	gate := newTestLiveGate(liveSpendSetupTokens(t) + liveDefaultTurnOutputTokens + 2 + 3*perChunk)
	endpoint, results := liveFixtureWithGate(t, channeltype.Gemini, "gemini-3.8-live", gate, func(up *websocket.Conn) error {
		if err := acknowledgeLiveFixture(up, "gemini-3.8-live"); err != nil {
			return err
		}
		if _, _, err := up.ReadMessage(); err != nil {
			return errors.Wrap(err, "read funded input")
		}
		for range 20 {
			if err := liveWrite(up, websocket.TextMessage, chunk, time.Second); err != nil {
				return nil
			}
		}
		return nil
	})
	client := connectLiveFixture(t, endpoint)
	require.NoError(t, client.WriteMessage(websocket.TextMessage, []byte(text)))
	received := 0
	reason := ""
	for {
		_, _, err := client.ReadMessage()
		if err != nil {
			var closeErr *websocket.CloseError
			if errors.As(err, &closeErr) {
				reason = closeErr.Text
			}
			break
		}
		received++
	}
	usage := receiveLiveFixture(t, results)
	require.Equal(t, 3, received, "only funded output frames are forwarded")
	require.Equal(t, liveCloseQuotaExhausted, reason)
	require.True(t, usage.Realtime.HasUsageGap(), "the cut turn stays an explicit usage gap")
	_, _, evidence := gate.snapshot()
	require.Len(t, evidence, 1)
	require.EqualValues(t, 4*perChunk, evidence[0].OutputAudio, "evidence keeps all output the provider streamed")
}
