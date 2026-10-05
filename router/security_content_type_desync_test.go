package router

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/channeltype"
)

// mislabeledJSONContentType is one way a client can label (or fail to label) a
// JSON request body with something other than application/json.
type mislabeledJSONContentType struct {
	name        string
	contentType string
}

// mislabeledJSONContentTypes lists the Content-Type spellings under which the
// relay used to bind its typed request from the URL query string while the raw
// JSON body was still what reached the provider. An empty contentType omits the
// header entirely. None of them names a form, so the body has one reading.
var mislabeledJSONContentTypes = []mislabeledJSONContentType{
	{"text_plain", "text/plain"},
	{"missing", ""},
	{"text_plain_charset", "text/plain; charset=utf-8"},
	{"json_mixed_case", "Application/JSON"},
}

// formLabeledJSONContentType labels a JSON body as a urlencoded form. The
// provider receives that label, and a JSON object can also parse as a form, so
// the gateway must refuse the request instead of guessing which one it meant.
const formLabeledJSONContentType = "application/x-www-form-urlencoded"

// relayOutcome is what one relay request did: the client status, the user
// ledger delta, the consume-log models and the raw bodies the provider got.
type relayOutcome struct {
	Code      int
	Charged   int64
	Billed    []string
	Forwarded []string
}

// relayAs sends one authenticated relay request through engine with an
// explicit raw query and Content-Type (omitted when empty), then waits for
// detached billing so ledger reads are final.
func relayAs(t *testing.T, engine http.Handler, key, path, rawQuery, contentType, body string) *httptest.ResponseRecorder {
	t.Helper()
	target := path
	if rawQuery != "" {
		target += "?" + rawQuery
	}
	req := httptest.NewRequest(http.MethodPost, target, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer sk-"+key)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	drainCriticalTasksForRouter(t)
	return w
}

// relayFixture builds a fresh shipped router and provider for one request and
// returns the engine, the token key and a snapshot of the provider bodies.
type relayFixture func(t *testing.T) (http.Handler, string, func() [][]byte)

// runRelayOutcome executes one request on a fresh fixture and records its outcome.
func runRelayOutcome(t *testing.T, fixture relayFixture, path, rawQuery, contentType, body string) relayOutcome {
	t.Helper()
	engine, key, forwarded := fixture(t)
	before, _ := passthroughKeysLedger(t)
	w := relayAs(t, engine, key, path, rawQuery, contentType, body)
	after, billed := passthroughKeysLedger(t)
	out := relayOutcome{Code: w.Code, Charged: before - after, Billed: billed}
	for _, raw := range forwarded() {
		out.Forwarded = append(out.Forwarded, string(raw))
	}
	return out
}

// requireSameOutcomeForEveryContentType runs the JSON baseline, lets check
// assert it, then requires every mislabeled Content-Type to produce exactly
// the same status, ledger movement, billed models and provider bodies. The
// query string is attacker controlled and must never change any of them. A
// form-labeled JSON body must instead be rejected before any dispatch or charge.
func requireSameOutcomeForEveryContentType(t *testing.T, fixture relayFixture, path, rawQuery, body string, check func(t *testing.T, baseline relayOutcome)) {
	t.Helper()
	baseline := runRelayOutcome(t, fixture, path, rawQuery, "application/json", body)
	check(t, baseline)
	for _, tc := range mislabeledJSONContentTypes {
		t.Run(tc.name, func(t *testing.T) {
			got := runRelayOutcome(t, fixture, path, rawQuery, tc.contentType, body)
			require.Equal(t, baseline, got,
				"Content-Type %q changed the outcome: the provider received %q and the ledger charged %d for %v (status %d); application/json gives status %d, charge %d for %v",
				tc.contentType, got.Forwarded, got.Charged, got.Billed, got.Code, baseline.Code, baseline.Charged, baseline.Billed)
		})
	}
	t.Run("form_urlencoded_rejected", func(t *testing.T) {
		got := runRelayOutcome(t, fixture, path, rawQuery, formLabeledJSONContentType, body)
		require.Equal(t, http.StatusBadRequest, got.Code, "a JSON body labeled %q must be refused as a client error", formLabeledJSONContentType)
		require.Empty(t, got.Forwarded, "a refused request must not reach the provider")
		require.Zero(t, got.Charged)
		require.Empty(t, got.Billed)
	})
}

// setLedgerBalance sets the fixture user's balance and token remaining quota.
func setLedgerBalance(t *testing.T, quota int64) {
	t.Helper()
	require.NoError(t, model.DB.Model(&model.User{}).Where("id = ?", passthroughKeysUserID).Update("quota", quota).Error)
	require.NoError(t, model.DB.Model(&model.Token{}).Where("id = ?", passthroughKeysTokenID).Update("remain_quota", quota).Error)
}

// TestSecurityChatMislabeledContentTypeCannotSplitModel drives
// POST /v1/chat/completions through the shipped router (token auth,
// distributor, relay controller, SQLite ledger) onto the raw OpenAI-compatible
// passthrough branch. The token, channel and ledger admit only gpt-4o-mini.
// The JSON body asks for gpt-4o; the query string names gpt-4o-mini under the
// form-binding spellings (model for the distributor probe, Model and Messages
// for the typed chat request). Whatever the Content-Type, the gateway must
// refuse it like application/json (403: model not allowed) instead of
// authorizing, routing and billing the query while the provider serves the body.
func TestSecurityChatMislabeledContentTypeCannotSplitModel(t *testing.T) {
	fixture := func(t *testing.T) (http.Handler, string, func() [][]byte) {
		upstream, capture := newExtraBodyGateway(t)
		engine, key := passthroughKeysRouter(t, upstream.URL)
		client.HTTPClient = upstream.Client()
		return engine, key, func() [][]byte { bodies, _, _ := capture.snapshot(); return bodies }
	}
	query := "model=" + passthroughKeysAllowedModel + "&Model=" + passthroughKeysAllowedModel +
		"&Messages=" + url.QueryEscape(`{"role":"user","content":"hi"}`)
	body := `{"model":"` + passthroughKeysPremiumModel + `","messages":[{"role":"user","content":"hello"}],"max_tokens":32000}`

	requireSameOutcomeForEveryContentType(t, fixture, "/v1/chat/completions", query, body, func(t *testing.T, baseline relayOutcome) {
		require.Equal(t, http.StatusForbidden, baseline.Code)
		require.Empty(t, baseline.Forwarded)
		require.Zero(t, baseline.Charged)
	})
}

// TestSecurityResponsesMislabeledContentTypeCannotSkipReservation drives
// POST /v1/responses onto the native Responses upstream. The balance covers a
// one-token prompt but not the body's max_output_tokens, so application/json
// is refused before dispatch. A query-bound typed request (a prompt template
// selected by Version, no max_output_tokens) must not reserve for itself while
// the provider receives the body's input and max_output_tokens.
func TestSecurityResponsesMislabeledContentTypeCannotSkipReservation(t *testing.T) {
	fixture := func(t *testing.T) (http.Handler, string, func() [][]byte) {
		upstream, capture := newExactKeyUpstream(t, func(w http.ResponseWriter, _ map[string]json.RawMessage) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"id":"resp_fixture","object":"response","created_at":1767225600,"status":"completed","model":"`+responsesNativeModel+`","output":[{"type":"message","id":"msg_fixture","status":"completed","role":"assistant","content":[{"type":"output_text","text":"ok","annotations":[]}]}],"usage":{"input_tokens":5,"output_tokens":2,"total_tokens":7}}`)
		})
		engine, key := ambiguousKeysRouter(t, upstream.URL, channeltype.XAI, responsesNativeModel, true)
		client.HTTPClient = upstream.Client()
		setLedgerBalance(t, 20_000)
		return engine, key, capture.snapshot
	}
	query := "model=" + responsesNativeModel + "&Model=" + responsesNativeModel + "&Version=1"
	body := `{"model":"` + responsesNativeModel + `","input":"` + strings.Repeat("expensive prompt ", 64) + `","max_output_tokens":200000}`

	requireSameOutcomeForEveryContentType(t, fixture, "/v1/responses", query, body, func(t *testing.T, baseline relayOutcome) {
		require.Equal(t, http.StatusForbidden, baseline.Code, "the balance must not cover the body's max_output_tokens")
		require.Empty(t, baseline.Forwarded)
		require.Zero(t, baseline.Charged)
	})
}

// TestSecurityClaudeMislabeledContentTypeCannotSkipReservation drives
// POST /v1/messages onto the native Anthropic passthrough. The balance covers
// max_tokens=1 but not the body's max_tokens, so application/json is refused
// before dispatch. A query-bound typed request (MaxTokens=1, one short
// message) must not be admitted while the provider receives the body.
func TestSecurityClaudeMislabeledContentTypeCannotSkipReservation(t *testing.T) {
	const claudeModel = "claude-haiku-4-5"
	fixture := func(t *testing.T) (http.Handler, string, func() [][]byte) {
		upstream, capture := newExactKeyUpstream(t, func(w http.ResponseWriter, _ map[string]json.RawMessage) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"id":"msg_fixture","type":"message","role":"assistant","model":"`+claudeModel+`","content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn","stop_sequence":null,"usage":{"input_tokens":5,"output_tokens":2}}`)
		})
		engine, key := ambiguousKeysRouter(t, upstream.URL, channeltype.Anthropic, claudeModel, true)
		client.HTTPClient = upstream.Client()
		setLedgerBalance(t, 20_000)
		return engine, key, capture.snapshot
	}
	query := "model=" + claudeModel + "&Model=" + claudeModel + "&MaxTokens=1&Messages=" + url.QueryEscape(`{"role":"user","content":"hi"}`)
	body := `{"model":"` + claudeModel + `","max_tokens":64000,"messages":[{"role":"user","content":"` + strings.Repeat("expensive prompt ", 64) + `"}]}`

	requireSameOutcomeForEveryContentType(t, fixture, "/v1/messages", query, body, func(t *testing.T, baseline relayOutcome) {
		require.Equal(t, http.StatusForbidden, baseline.Code, "the balance must not cover the body's max_tokens")
		require.Empty(t, baseline.Forwarded)
		require.Zero(t, baseline.Charged)
	})
}

// TestSecuritySpeechMislabeledContentTypeBillsSynthesizedInput drives
// POST /v1/audio/speech onto an OpenAI-compatible provider. Speech is metered
// on the typed input while the wire body is built from the raw JSON, so a
// query-bound typed input ("hi") must not be billed while the provider
// synthesizes the long body input.
func TestSecuritySpeechMislabeledContentTypeBillsSynthesizedInput(t *testing.T) {
	fixture := func(t *testing.T) (http.Handler, string, func() [][]byte) {
		upstream, capture := newExactKeyUpstream(t, func(w http.ResponseWriter, _ map[string]json.RawMessage) {
			w.Header().Set("Content-Type", "audio/mpeg")
			_, _ = w.Write([]byte("ID3fixture-audio"))
		})
		engine, key := ambiguousKeysRouter(t, upstream.URL, channeltype.OpenAICompatible, "tts-1", true)
		client.HTTPClient = upstream.Client()
		return engine, key, capture.snapshot
	}
	long := strings.Repeat("metered speech text ", 200)
	query := "model=tts-1&Model=tts-1&Voice=alloy&Input=hi"
	body := `{"model":"tts-1","voice":"alloy","input":"` + long + `"}`

	requireSameOutcomeForEveryContentType(t, fixture, "/v1/audio/speech", query, body, func(t *testing.T, baseline relayOutcome) {
		require.Equal(t, http.StatusOK, baseline.Code)
		require.Len(t, baseline.Forwarded, 1)
		require.Contains(t, baseline.Forwarded[0], long)
		require.Positive(t, baseline.Charged)
	})
}

// TestSecurityImageMislabeledContentTypeBillsRenderedImages drives
// POST /v1/images/generations onto an OpenAI-compatible provider that receives
// the raw body. Billing reads the typed n and size, so a query-bound request
// (n=1, 256x256) must not be billed while the provider renders the body's four
// 1024x1024 images.
func TestSecurityImageMislabeledContentTypeBillsRenderedImages(t *testing.T) {
	fixture := func(t *testing.T) (http.Handler, string, func() [][]byte) {
		upstream, capture := newExactKeyUpstream(t, func(w http.ResponseWriter, root map[string]json.RawMessage) {
			count := 1
			if raw, ok := root["n"]; ok {
				_ = json.Unmarshal(raw, &count)
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
		return engine, key, capture.snapshot
	}
	query := "model=dall-e-2&prompt=a+cat&n=1&size=256x256"
	body := `{"model":"dall-e-2","prompt":"a cat","n":4,"size":"1024x1024"}`

	requireSameOutcomeForEveryContentType(t, fixture, "/v1/images/generations", query, body, func(t *testing.T, baseline relayOutcome) {
		require.Equal(t, http.StatusOK, baseline.Code)
		require.Equal(t, []string{body}, baseline.Forwarded)
		require.Positive(t, baseline.Charged)
	})
}

// TestSecurityVideoMislabeledContentTypeBillsRenderedSeconds drives
// POST /v1/videos onto an OpenAI-compatible provider that receives the raw
// body. Billing reads the typed duration, so a query-bound request (4 seconds)
// must not be billed while the provider renders the body's 12 seconds.
func TestSecurityVideoMislabeledContentTypeBillsRenderedSeconds(t *testing.T) {
	const videoModel = "sora-2"
	fixture := func(t *testing.T) (http.Handler, string, func() [][]byte) {
		upstream, capture := newExactKeyUpstream(t, func(w http.ResponseWriter, _ map[string]json.RawMessage) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"id":"video_fixture","object":"video","model":"`+videoModel+`","status":"queued","created_at":1767225600}`)
		})
		engine, key := ambiguousKeysRouter(t, upstream.URL, channeltype.OpenAICompatible, videoModel, true)
		client.HTTPClient = upstream.Client()
		channel := model.Channel{Id: 929293}
		require.NoError(t, channel.SetModelPriceConfigs(map[string]model.ModelConfigLocal{
			videoModel: {Video: &model.VideoPricingLocal{PerSecondUsd: 0.10}},
		}))
		require.NoError(t, model.DB.Model(&model.Channel{}).Where("id = ?", channel.Id).Update("model_configs", channel.ModelConfigs).Error)
		return engine, key, capture.snapshot
	}
	query := "model=" + videoModel + "&prompt=a+cat&seconds=4"
	body := `{"model":"` + videoModel + `","prompt":"a cat","seconds":12}`

	requireSameOutcomeForEveryContentType(t, fixture, "/v1/videos", query, body, func(t *testing.T, baseline relayOutcome) {
		require.Equal(t, http.StatusOK, baseline.Code)
		require.Equal(t, []string{body}, baseline.Forwarded)
		require.Positive(t, baseline.Charged)
	})
}

// TestSecurityImageMappedModelReachesProviderForEveryJSONLabel drives
// POST /v1/images/generations through a channel that maps dall-e-2 to
// dall-e-3. Billing prices the mapped model, so whenever the gateway decoded
// the body as JSON the provider must receive the mapped model too, including
// JSON sent with parameters, mixed case, text/plain or no Content-Type.
func TestSecurityImageMappedModelReachesProviderForEveryJSONLabel(t *testing.T) {
	body := `{"model":"dall-e-2","prompt":"a cat","n":1,"size":"1024x1024"}`
	labels := append([]mislabeledJSONContentType{{"json", "application/json"}, {"json_charset", "application/json; charset=utf-8"}}, mislabeledJSONContentTypes...)
	for _, tc := range labels {
		t.Run(tc.name, func(t *testing.T) {
			upstream, capture := newExactKeyUpstream(t, func(w http.ResponseWriter, _ map[string]json.RawMessage) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `{"created":1767225600,"data":[{"url":"https://images.example/cat.png"}]}`)
			})
			engine, key := ambiguousKeysRouter(t, upstream.URL, channeltype.OpenAICompatible, "dall-e-2", true)
			client.HTTPClient = upstream.Client()
			require.NoError(t, model.DB.Model(&model.Channel{}).Where("id = ?", 929293).Update("model_mapping", `{"dall-e-2":"dall-e-3"}`).Error)

			w := relayAs(t, engine, key, "/v1/images/generations", "", tc.contentType, body)
			require.Equal(t, http.StatusOK, w.Code, w.Body.String())
			bodies := capture.snapshot()
			require.Len(t, bodies, 1)
			var forwarded struct {
				Model string `json:"model"`
			}
			require.NoError(t, json.Unmarshal(bodies[0], &forwarded))
			require.Equal(t, "dall-e-3", forwarded.Model, "the provider must render the model billing priced")
		})
	}
}
