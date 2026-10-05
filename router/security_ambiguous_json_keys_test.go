package router

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/relay/channeltype"
)

// exactKeyUpstream is a case-sensitive provider fixture: like Python, Node and
// most OpenAI-compatible servers it reads object keys exactly and ignores
// unknown spellings. It records every raw body it receives.
type exactKeyUpstream struct {
	mu     sync.Mutex
	bodies [][]byte
}

// snapshot returns a copy of the recorded raw bodies.
func (u *exactKeyUpstream) snapshot() [][]byte {
	u.mu.Lock()
	defer u.mu.Unlock()
	return append([][]byte(nil), u.bodies...)
}

// newExactKeyUpstream starts the fixture; respond writes the provider reply
// for the decoded exact-key root of each request.
func newExactKeyUpstream(t *testing.T, respond func(w http.ResponseWriter, root map[string]json.RawMessage)) (*httptest.Server, *exactKeyUpstream) {
	t.Helper()
	capture := &exactKeyUpstream{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "read failed", http.StatusBadRequest)
			return
		}
		capture.mu.Lock()
		capture.bodies = append(capture.bodies, raw)
		capture.mu.Unlock()
		var root map[string]json.RawMessage
		if err := json.Unmarshal(raw, &root); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		respond(w, root)
	}))
	t.Cleanup(server.Close)
	return server, capture
}

// postJSON sends one authenticated JSON request through the engine.
func postJSON(t *testing.T, engine http.Handler, key, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer sk-"+key)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	drainCriticalTasksForRouter(t)
	return w
}

// responsesNativeModel is served through the native Responses upstream path.
const responsesNativeModel = "grok-4.6"

// TestSecurityResponsesAmbiguousPreviousResponseID shows that a case-folded
// null previous_response_id must not hide the exact-key value from the
// ownership check. Go's decoder keeps the last (null) member, the check is
// skipped, and the raw provider handle would reach the upstream unverified.
func TestSecurityResponsesAmbiguousPreviousResponseID(t *testing.T) {
	const foreign = "resp_foreign_upstream_handle"
	for _, tc := range []struct {
		name string
		body string
	}{
		{"plain_foreign_id", `{"model":"` + responsesNativeModel + `","input":"hello","previous_response_id":"` + foreign + `"}`},
		{"case_folded_null_shadow", `{"model":"` + responsesNativeModel + `","input":"hello","previous_response_id":"` + foreign + `","Previous_Response_Id":null}`},
		{"escaped_null_shadow", `{"model":"` + responsesNativeModel + `","input":"hello","previous_response_id":"` + foreign + `","` + jsonEscaped("p") + `revious_response_id":null}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			upstream, capture := newExactKeyUpstream(t, func(w http.ResponseWriter, _ map[string]json.RawMessage) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `{"id":"resp_fixture","object":"response","created_at":1767225600,"status":"completed","model":"`+responsesNativeModel+`","output":[{"type":"message","id":"msg_fixture","status":"completed","role":"assistant","content":[{"type":"output_text","text":"ok","annotations":[]}]}],"usage":{"input_tokens":5,"output_tokens":2,"total_tokens":7}}`)
			})
			engine, key := ambiguousKeysRouter(t, upstream.URL, channeltype.XAI, responsesNativeModel, true)
			client.HTTPClient = upstream.Client()

			w := postJSON(t, engine, key, "/v1/responses", tc.body)

			for _, body := range capture.snapshot() {
				var root map[string]json.RawMessage
				require.NoError(t, json.Unmarshal(body, &root))
				require.NotContains(t, string(root["previous_response_id"]), foreign,
					"an unowned provider handle must never reach the upstream (status %d): %s", w.Code, body)
			}
			require.GreaterOrEqual(t, w.Code, http.StatusBadRequest, w.Body.String())
		})
	}
}

// TestSecurityImageAmbiguousCountBilledAsRequested shows that an OpenAI-
// compatible image request cannot be billed for one image (or a cheaper size
// tier) while the raw body asks a case-sensitive upstream for four images (or
// leaves it to render its larger default size).
func TestSecurityImageAmbiguousCountBilledAsRequested(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
	}{
		{"case_folded_n", `{"model":"dall-e-2","prompt":"a cat","size":"256x256","n":4,"N":1}`},
		{"exact_duplicate_n", `{"model":"dall-e-2","prompt":"a cat","size":"256x256","n":4,"n":1}`},
		// Billed at the 256x256 tier while an exact-key provider never sees
		// the size and renders its 1024x1024 default.
		{"lone_case_folded_size", `{"model":"dall-e-2","prompt":"a cat","Size":"256x256"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			upstream, capture := newExactKeyUpstream(t, func(w http.ResponseWriter, root map[string]json.RawMessage) {
				count := 1
				if raw, ok := root["n"]; ok {
					require.NoError(t, json.Unmarshal(raw, &count))
				}
				items := make([]string, 0, count)
				for range count {
					items = append(items, `{"url":"https://images.example/cat.png"}`)
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `{"created":1767225600,"data":[`+strings.Join(items, ",")+`]}`)
			})
			engine, key := ambiguousKeysRouter(t, upstream.URL, channeltype.OpenAICompatible, "dall-e-2", true)
			client.HTTPClient = upstream.Client()
			before, _ := passthroughKeysLedger(t)

			w := postJSON(t, engine, key, "/v1/images/generations", tc.body)

			after, models := passthroughKeysLedger(t)
			bodies := capture.snapshot()
			require.Equal(t, http.StatusBadRequest, w.Code,
				"ambiguous image count must be rejected; upstream got %q, ledger moved %d for %v", bodies, before-after, models)
			require.Empty(t, bodies)
			require.Equal(t, before, after)
		})
	}
}

// TestSecuritySpeechAmbiguousInputBilledAsSynthesized shows that a speech
// request cannot be metered on a short case-folded input while a
// case-sensitive upstream synthesizes the long exact-key input.
func TestSecuritySpeechAmbiguousInputBilledAsSynthesized(t *testing.T) {
	long := strings.Repeat("metered speech text ", 200)
	upstream, capture := newExactKeyUpstream(t, func(w http.ResponseWriter, _ map[string]json.RawMessage) {
		w.Header().Set("Content-Type", "audio/mpeg")
		_, _ = w.Write([]byte("ID3fixture-audio"))
	})
	engine, key := ambiguousKeysRouter(t, upstream.URL, channeltype.OpenAICompatible, "tts-1", true)
	client.HTTPClient = upstream.Client()
	before, _ := passthroughKeysLedger(t)

	w := postJSON(t, engine, key, "/v1/audio/speech", `{"model":"tts-1","voice":"alloy","input":"`+long+`","Input":"hi"}`)

	after, models := passthroughKeysLedger(t)
	bodies := capture.snapshot()
	var synthesized []string
	for _, body := range bodies {
		var root map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(body, &root))
		synthesized = append(synthesized, string(root["input"]))
	}
	require.Equal(t, http.StatusBadRequest, w.Code,
		"ambiguous speech input must be rejected; upstream synthesized %d input(s) of %v bytes while the ledger moved %d for %v",
		len(bodies), lengths(synthesized), before-after, models)
	require.Empty(t, bodies)
	require.Equal(t, before, after)
}

// lengths returns the byte length of each string, for compact diagnostics.
func lengths(values []string) []int {
	out := make([]int, 0, len(values))
	for _, value := range values {
		out = append(out, len(value))
	}
	return out
}
