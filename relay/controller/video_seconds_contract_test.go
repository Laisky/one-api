package controller

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/channeltype"
)

// videoSecondsWire is one provider request observed by the fake OpenAI
// Videos endpoint. It is captured on the server goroutine and asserted later.
type videoSecondsWire struct {
	method, path, contentType string
	body                      []byte
}

// videoSecondsProvider is a fake OpenAI Videos provider that accepts every
// creation and records each wire request it receives.
type videoSecondsProvider struct {
	server *httptest.Server
	calls  atomic.Int32
	mu     sync.Mutex
	wires  []videoSecondsWire
}

// newVideoSecondsProvider starts a TLS fake provider that answers every
// creation with an accepted job whose id is taskID, and routes the shared HTTP
// client to it for the duration of the test.
//
// Parameters:
//   - t: the running test; cleanup closes the server and restores the client.
//   - taskID: the provider job id returned to the gateway.
//
// Returns:
//   - *videoSecondsProvider: the running provider and its wire log.
func newVideoSecondsProvider(t *testing.T, taskID string) *videoSecondsProvider {
	t.Helper()
	p := &videoSecondsProvider{}
	p.server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p.calls.Add(1)
		body, err := io.ReadAll(r.Body)
		if err != nil {
			body = []byte("read error: " + err.Error())
		}
		p.mu.Lock()
		p.wires = append(p.wires, videoSecondsWire{method: r.Method, path: r.URL.Path, contentType: r.Header.Get("Content-Type"), body: body})
		p.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprintf(w, `{"id":%q,"object":"video","model":"sora-2","status":"queued"}`, taskID)
	}))
	previous := client.HTTPClient
	client.HTTPClient = p.server.Client()
	t.Cleanup(func() {
		client.HTTPClient = previous
		p.server.Close()
	})
	return p
}

// onlyWire returns the single recorded provider request, failing the test when
// the provider was not called exactly once.
//
// Parameters:
//   - t: the running test.
//
// Returns:
//   - videoSecondsWire: the one observed provider request.
func (p *videoSecondsProvider) onlyWire(t *testing.T) videoSecondsWire {
	t.Helper()
	require.EqualValues(t, 1, p.calls.Load(), "exactly one provider creation")
	p.mu.Lock()
	defer p.mu.Unlock()
	require.Len(t, p.wires, 1)
	return p.wires[0]
}

// videoSecondsMultipart encodes fields as a multipart/form-data body the way
// the official OpenAI SDKs send POST /v1/videos.
//
// Parameters:
//   - t: the running test.
//   - fields: ordered name/value pairs to write as form fields.
//
// Returns:
//   - string: the encoded body.
//   - string: the Content-Type header carrying the boundary.
func videoSecondsMultipart(t *testing.T, fields [][2]string) (string, string) {
	t.Helper()
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	for _, field := range fields {
		require.NoError(t, writer.WriteField(field[0], field[1]))
	}
	require.NoError(t, writer.Close())
	return buf.String(), writer.FormDataContentType()
}

// videoSecondsForm decodes a forwarded multipart body into its text fields.
//
// Parameters:
//   - t: the running test.
//   - wire: the observed provider request.
//
// Returns:
//   - map[string][]string: the form fields the provider received.
func videoSecondsForm(t *testing.T, wire videoSecondsWire) map[string][]string {
	t.Helper()
	mediaType, params, err := mime.ParseMediaType(wire.contentType)
	require.NoError(t, err)
	require.Equal(t, "multipart/form-data", mediaType)
	form, err := multipart.NewReader(bytes.NewReader(wire.body), params["boundary"]).ReadForm(1 << 20)
	require.NoError(t, err)
	return form.Value
}

// requireVideoLedgerUntouched asserts that a rejected request reserved and
// charged nothing: balances are unchanged and no request cost was recorded.
//
// Parameters:
//   - t: the running test.
//   - balance: the user and token balance before the request.
//   - requestID: the rejected request's id.
//
// Returns: none.
func requireVideoLedgerUntouched(t *testing.T, balance int64, requestID string) {
	t.Helper()
	require.Equal(t, balance, reloadUserQuota(t), "user ledger")
	var token model.Token
	require.NoError(t, model.DB.First(&token, fallbackTokenID).Error)
	require.Equal(t, balance, token.RemainQuota, "token remaining quota")
	require.Zero(t, token.UsedQuota, "token used quota")
	var charged int64
	require.NoError(t, model.DB.Model(&model.UserRequestCost{}).Where("request_id = ? AND quota <> 0", requestID).Count(&charged).Error)
	require.Zero(t, charged, "request cost ledger")
}

// TestVideoSecondsAcceptedFormsBillAndForward drives the real RelayVideoHelper
// against a fake OpenAI provider. The documented string seconds ("4", "8",
// "12"), the legacy JSON number and the multipart form the official SDKs send
// must all be admitted, charged exactly duration x per-second price (string
// and number identically), and forwarded with the client's own representation.
//
// Parameters:
//   - t: the running test.
//
// Returns: none.
func TestVideoSecondsAcceptedFormsBillAndForward(t *testing.T) {
	for _, tc := range []struct {
		name, actual, body string
		multipart          [][2]string
		charge             int64
		// wantSeconds is the raw JSON token (or form value) the provider must see.
		wantSeconds string
	}{
		{name: "json_string_4", actual: "sora-2", body: `{"model":"alias","prompt":"A paper boat.","seconds":"4"}`, charge: 200000, wantSeconds: `"4"`},
		{name: "json_number_4", actual: "sora-2", body: `{"model":"alias","prompt":"A paper boat.","seconds":4}`, charge: 200000, wantSeconds: `4`},
		{name: "json_string_8", actual: "sora-2", body: `{"model":"alias","prompt":"A paper boat.","seconds":"8","size":"1280x720"}`, charge: 400000, wantSeconds: `"8"`},
		{name: "json_string_12", actual: "sora-2", body: `{"model":"alias","prompt":"A paper boat.","seconds":"12"}`, charge: 600000, wantSeconds: `"12"`},
		{name: "json_number_12", actual: "sora-2", body: `{"model":"alias","prompt":"A paper boat.","seconds":12}`, charge: 600000, wantSeconds: `12`},
		// $0.50/s at 1792x1024 for 12 s is $6.00; the legacy multiplier's
		// binary noise rounds up one unit, identically for both forms.
		{name: "pro_string_12_1792", actual: "sora-2-pro", body: `{"model":"alias","prompt":"A paper boat.","seconds":"12","size":"1792x1024"}`, charge: 3000001, wantSeconds: `"12"`},
		{name: "pro_number_12_1792", actual: "sora-2-pro", body: `{"model":"alias","prompt":"A paper boat.","seconds":12,"size":"1792x1024"}`, charge: 3000001, wantSeconds: `12`},
		{name: "equal_aliases", actual: "sora-2", body: `{"model":"alias","prompt":"A paper boat.","seconds":"8","duration":8}`, charge: 400000, wantSeconds: `"8"`},
		{name: "multipart_sdk_8", actual: "sora-2", multipart: [][2]string{{"model", "sora-2"}, {"prompt", "A paper boat."}, {"seconds", "8"}, {"size", "720x1280"}}, charge: 400000, wantSeconds: "8"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			const balance = int64(10_000_000)
			xaiVideoSetup(t, balance, false)
			taskID := "video_vsec_" + tc.name
			provider := newVideoSecondsProvider(t, taskID)
			body, contentType := tc.body, "application/json"
			if tc.multipart != nil {
				body, contentType = videoSecondsMultipart(t, tc.multipart)
			}
			c, w, requestID := protocolContext(t, channeltype.OpenAI, tc.actual, "/v1/videos", body, provider.server.URL, balance, 1, false, nil)
			c.Request.Header.Set("Content-Type", contentType)
			c.Set(ctxkey.ContentType, contentType)

			apiErr := RelayVideoHelper(c)
			drainCriticalTasks(t)

			require.Nil(t, apiErr)
			require.Equal(t, http.StatusOK, w.Code)
			requireOpenAIVideoLedger(t, balance, tc.charge, requestID)
			wire := provider.onlyWire(t)
			require.Equal(t, http.MethodPost+" /v1/videos", wire.method+" "+wire.path)
			if tc.multipart != nil {
				require.Equal(t, []byte(body), wire.body, "multipart body forwarded byte-identically")
				form := videoSecondsForm(t, wire)
				require.Equal(t, []string{tc.wantSeconds}, form["seconds"])
				return
			}
			var forwarded map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(wire.body, &forwarded))
			require.Equal(t, tc.wantSeconds, string(forwarded["seconds"]), "client seconds representation preserved")
			require.JSONEq(t, fmt.Sprintf("%q", tc.actual), string(forwarded["model"]), "mapped model forwarded")
		})
	}
}

// TestVideoSecondsInvalidRejectedBeforeDispatch proves an unparseable,
// non-positive, non-decimal, conflicting or missing duration is refused with
// 400 before any quota reservation or provider call, rather than defaulted or
// billed from a different alias than the one the provider reads.
//
// Parameters:
//   - t: the running test.
//
// Returns: none.
func TestVideoSecondsInvalidRejectedBeforeDispatch(t *testing.T) {
	for _, tc := range []struct {
		name, body, code string
		// msg is a substring of the refusal that names the failed rule.
		msg       string
		multipart [][2]string
	}{
		{name: "string_unparseable", body: `{"model":"alias","prompt":"p","seconds":"abc"}`, code: "invalid_video_request", msg: "decimal seconds"},
		{name: "string_empty", body: `{"model":"alias","prompt":"p","seconds":""}`, code: "invalid_video_request"},
		{name: "string_zero", body: `{"model":"alias","prompt":"p","seconds":"0"}`, code: "invalid_video_duration", msg: "positive number of seconds"},
		{name: "string_negative", body: `{"model":"alias","prompt":"p","seconds":"-4"}`, code: "invalid_video_request"},
		{name: "string_padded", body: `{"model":"alias","prompt":"p","seconds":" 4"}`, code: "invalid_video_request"},
		{name: "string_nan", body: `{"model":"alias","prompt":"p","seconds":"NaN"}`, code: "invalid_video_request", msg: "decimal seconds"},
		{name: "string_infinity", body: `{"model":"alias","prompt":"p","seconds":"Infinity"}`, code: "invalid_video_request"},
		{name: "string_hex", body: `{"model":"alias","prompt":"p","seconds":"0x10"}`, code: "invalid_video_request"},
		{name: "string_exponent", body: `{"model":"alias","prompt":"p","seconds":"4e1"}`, code: "invalid_video_request"},
		{name: "string_overlong", body: `{"model":"alias","prompt":"p","seconds":"1234567890123"}`, code: "invalid_video_request"},
		{name: "bool", body: `{"model":"alias","prompt":"p","seconds":true}`, code: "invalid_video_request", msg: "must be a number or a decimal string"},
		{name: "object", body: `{"model":"alias","prompt":"p","seconds":{}}`, code: "invalid_video_request"},
		{name: "number_zero", body: `{"model":"alias","prompt":"p","seconds":0}`, code: "invalid_video_duration", msg: "positive number of seconds"},
		{name: "number_negative", body: `{"model":"alias","prompt":"p","seconds":-4}`, code: "invalid_video_duration", msg: "positive number of seconds"},
		{name: "number_zero_with_alias", body: `{"model":"alias","prompt":"p","seconds":0,"duration":8}`, code: "invalid_video_duration", msg: "positive number of seconds"},
		{name: "conflict_string_vs_alias", body: `{"model":"alias","prompt":"p","seconds":"4","duration_seconds":12}`, code: "invalid_video_duration", msg: "conflicting video duration fields"},
		// The provider reads seconds (12) while the old priority billed
		// duration_seconds (4): a 3x under-charge.
		{name: "conflict_number_underbill", body: `{"model":"alias","prompt":"p","seconds":12,"duration_seconds":4}`, code: "invalid_video_duration", msg: "conflicting video duration fields"},
		{name: "omitted", body: `{"model":"alias","prompt":"p"}`, code: "invalid_video_duration", msg: "seconds must be positive for video generation"},
		{name: "null_is_omitted", body: `{"model":"alias","prompt":"p","seconds":null}`, code: "invalid_video_duration"},
		{name: "multipart_unparseable", multipart: [][2]string{{"model", "sora-2"}, {"prompt", "p"}, {"seconds", "abc"}}, code: "invalid_video_request", msg: "parse video request"},
		{name: "multipart_hex", multipart: [][2]string{{"model", "sora-2"}, {"prompt", "p"}, {"seconds", "0x10"}}, code: "invalid_video_request"},
		{name: "multipart_empty", multipart: [][2]string{{"model", "sora-2"}, {"prompt", "p"}, {"seconds", ""}}, code: "invalid_video_request"},
		{name: "multipart_conflict", multipart: [][2]string{{"model", "sora-2"}, {"prompt", "p"}, {"seconds", "12"}, {"duration_seconds", "4"}}, code: "invalid_video_duration", msg: "conflicting video duration fields"},
		// The official SDKs omit seconds to request OpenAI's documented 4 s
		// default. OpenAI retired its Videos API on 2026-09-24, and the hosts
		// still behind this route may render another default, so the gateway
		// refuses rather than bill a guessed length.
		{name: "multipart_omitted_sdk_default", multipart: [][2]string{{"model", "sora-2"}, {"prompt", "p"}, {"size", "720x1280"}}, code: "invalid_video_duration", msg: "seconds must be positive for video generation"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			const balance = int64(10_000_000)
			xaiVideoSetup(t, balance, false)
			provider := newVideoSecondsProvider(t, "video_vsec_rejected")
			body, contentType := tc.body, "application/json"
			if tc.multipart != nil {
				body, contentType = videoSecondsMultipart(t, tc.multipart)
			}
			bindingsBefore := countVideoTaskBindings(t)
			c, w, requestID := protocolContext(t, channeltype.OpenAI, "sora-2", "/v1/videos", body, provider.server.URL, balance, 1, false, nil)
			c.Request.Header.Set("Content-Type", contentType)
			c.Set(ctxkey.ContentType, contentType)

			apiErr := RelayVideoHelper(c)
			drainCriticalTasks(t)

			require.NotNil(t, apiErr, "invalid duration must be refused")
			require.Equal(t, http.StatusBadRequest, apiErr.StatusCode)
			require.Equal(t, tc.code, apiErr.Code)
			require.Contains(t, apiErr.Message, tc.msg)
			require.False(t, apiErr.FromUpstream, "refused locally, not by the provider")
			require.Zero(t, provider.calls.Load(), "no provider dispatch")
			require.False(t, c.Writer.Written())
			require.Empty(t, w.Body.String())
			requireVideoLedgerUntouched(t, balance, requestID)
			require.Equal(t, bindingsBefore, countVideoTaskBindings(t), "no task persisted")
		})
	}
}
