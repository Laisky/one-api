package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/ctxkey"
	dbmodel "github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/asyncvideo"
)

// asyncVideoSecondsContext builds a routed MuAPI POST /v1/async/videos request
// with a unique request id, so the request-cost ledger assertions never see
// rows left behind by other tests sharing the fixture user.
//
// Parameters:
//   - t: the running test.
//   - server: the fake MuAPI provider the channel points at.
//   - body: the client JSON body.
//   - balance: the user and token balance before the request.
//   - requestID: the unique request id for this admission.
//
// Returns:
//   - *gin.Context: the request context, ready for RelayAsyncVideoHelper.
//   - *httptest.ResponseRecorder: the recorder holding the rendered response.
func asyncVideoSecondsContext(t *testing.T, server *muapiTaskServer, body string, balance int64, requestID string) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	c, recorder := muapiVideoContext(t, http.MethodPost, "/v1/async/videos", body, server.URL, balance, fallbackUserID)
	c.Request.Header.Set("Idempotency-Key", requestID)
	c.Set(ctxkey.RequestId, requestID)
	return c, recorder
}

// countAsyncVideoTasks returns the number of durable async task rows.
//
// Parameters:
//   - t: the running test.
//
// Returns:
//   - int64: the row count of the async task table.
func countAsyncVideoTasks(t *testing.T) int64 {
	t.Helper()
	var count int64
	require.NoError(t, dbmodel.DB.Model(&dbmodel.AsyncTask{}).Count(&count).Error)
	return count
}

// TestAsyncVideoSecondsRefusedBeforeQuote drives the real durable async
// controller with invalid, conflicting, ambiguous or omitted durations. Each is
// refused with 400 before the provider is asked for a quote, before any quota
// is reserved and before a task row exists. Conflicts and non-positive values
// are refused by the decoder shared with POST /v1/videos (same code and rule),
// not left to whichever provider preparer happens to be routed.
//
// Parameters:
//   - t: the running test.
//
// Returns: none.
func TestAsyncVideoSecondsRefusedBeforeQuote(t *testing.T) {
	for _, tc := range []struct {
		name, body, code string
		// msg is a substring of the refusal that names the failed rule.
		msg string
	}{
		// A provider reading `seconds` renders 12 s while the old billing
		// priority resolved duration_seconds (4 s): a 3x under-charge.
		{name: "conflict_number_underbill", body: `{"model":"alias","prompt":"a lighthouse","seconds":12,"duration_seconds":4}`, code: "invalid_video_duration", msg: "conflicting video duration fields"},
		{name: "conflict_duration_vs_seconds", body: `{"model":"alias","prompt":"a lighthouse","duration":5,"seconds":3}`, code: "invalid_video_duration", msg: "conflicting video duration fields"},
		{name: "conflict_string_vs_number", body: `{"model":"alias","prompt":"a lighthouse","duration":5,"seconds":"3"}`, code: "invalid_video_duration", msg: "conflicting video duration fields"},
		{name: "number_zero", body: `{"model":"alias","prompt":"a lighthouse","duration":0}`, code: "invalid_video_duration", msg: "positive number of seconds"},
		{name: "number_negative", body: `{"model":"alias","prompt":"a lighthouse","seconds":-5}`, code: "invalid_video_duration", msg: "positive number of seconds"},
		{name: "string_zero", body: `{"model":"alias","prompt":"a lighthouse","seconds":"0"}`, code: "invalid_video_duration", msg: "positive number of seconds"},
		{name: "string_unparseable", body: `{"model":"alias","prompt":"a lighthouse","seconds":"abc"}`, code: "invalid_async_video_request", msg: "decimal seconds"},
		{name: "key_case_fold", body: `{"model":"alias","prompt":"a lighthouse","duration":5,"Duration":3}`, code: "invalid_async_video_request", msg: `must be spelled "duration"`},
		{name: "missing_model", body: `{"prompt":"a lighthouse","duration":5}`, code: "invalid_async_video_request", msg: "requires a JSON object with model"},
		// No default is invented: MuAPI's native contract requires a duration.
		{name: "omitted", body: `{"model":"alias","prompt":"a lighthouse"}`, code: "invalid_async_video_request", msg: "requires a positive integer duration"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			const balance = int64(10_000_000)
			muapiTaskSetup(t, balance)
			server := newMuapiTaskServer(t)
			requestID := "async-vsec-" + tc.name
			c, _ := asyncVideoSecondsContext(t, server, tc.body, balance, requestID)

			apiErr := RelayAsyncVideoHelper(c, false)
			drainCriticalTasks(t)

			require.NotNil(t, apiErr, "invalid duration must be refused")
			require.Equal(t, http.StatusBadRequest, apiErr.StatusCode)
			require.Equal(t, tc.code, apiErr.Code)
			require.Contains(t, apiErr.Message, tc.msg)
			require.False(t, apiErr.FromUpstream, "refused locally, not by the provider")
			require.False(t, c.Writer.Written(), "the relay renders the refusal")
			require.Zero(t, server.estimates.Load(), "no provider quote")
			require.Zero(t, server.creates.Load(), "no provider creation")
			require.Zero(t, countAsyncVideoTasks(t), "no durable task")
			require.Empty(t, c.GetString(asyncvideo.DurableTaskKey), "no task bound to the request")
			requireVideoLedgerUntouched(t, balance, requestID)
		})
	}
}

// TestAsyncVideoSecondsBillsProviderDuration admits valid durations through the
// durable controller and proves the reserved quota prices exactly the duration
// the provider is asked to render: the provider's own quote of the normalized
// body, or an administrator per-second override times the submitted duration.
// The worker then submits that same duration to the provider.
//
// Parameters:
//   - t: the running test.
//
// Returns: none.
func TestAsyncVideoSecondsBillsProviderDuration(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		// perSecondUsd, when positive, installs a channel per-second override
		// that replaces the provider quote.
		perSecondUsd float64
		charge       int64
		estimates    int32
	}{
		// MuAPI quotes $0.40 for the normalized 5 s body: 0.40 x 500000.
		{name: "seconds_number_quoted", body: `{"model":"alias","prompt":"a lighthouse","seconds":5}`, charge: 200000, estimates: 1},
		// The documented OpenAI string form is priced and submitted exactly like
		// the number: MuAPI receives the numeric duration it requires.
		{name: "seconds_string_quoted", body: `{"model":"alias","prompt":"a lighthouse","seconds":"5"}`, charge: 200000, estimates: 1},
		{name: "seconds_string_per_second_override", body: `{"model":"alias","prompt":"a lighthouse","seconds":"5"}`, perSecondUsd: 0.1, charge: 250000},
		{name: "agreeing_aliases_quoted", body: `{"model":"alias","prompt":"a lighthouse","duration":5,"seconds":5,"duration_seconds":5}`, charge: 200000, estimates: 1},
		// $0.10/s x 5 s = $0.50 = 250000 quota, priced from the submitted 5 s.
		{name: "seconds_per_second_override", body: `{"model":"alias","prompt":"a lighthouse","seconds":5}`, perSecondUsd: 0.1, charge: 250000},
		{name: "agreeing_aliases_per_second_override", body: `{"model":"alias","prompt":"a lighthouse","duration_seconds":5,"duration":5}`, perSecondUsd: 0.1, charge: 250000},
	} {
		t.Run(tc.name, func(t *testing.T) {
			const balance = int64(10_000_000)
			muapiTaskSetup(t, balance)
			server := newMuapiTaskServer(t)
			requestID := "async-vsec-ok-" + tc.name
			c, recorder := asyncVideoSecondsContext(t, server, tc.body, balance, requestID)
			if tc.perSecondUsd > 0 {
				value, ok := c.Get(ctxkey.ChannelModel)
				require.True(t, ok)
				channel, ok := value.(*dbmodel.Channel)
				require.True(t, ok)
				require.NoError(t, channel.SetModelPriceConfigs(map[string]dbmodel.ModelConfigLocal{
					"veo3-fast": {Video: &dbmodel.VideoPricingLocal{PerSecondUsd: tc.perSecondUsd}},
				}))
			}

			require.Nil(t, RelayAsyncVideoHelper(c, false))
			require.Equal(t, http.StatusAccepted, recorder.Code, recorder.Body.String())
			require.Equal(t, tc.estimates, server.estimates.Load())
			require.Equal(t, balance-tc.charge, reloadUserQuota(t), "reserved exactly the priced duration")

			var public asyncVideoTaskResponse
			require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &public))
			task, err := dbmodel.GetOwnedAsyncTask(t.Context(), public.ID, fallbackUserID, c.GetString(ctxkey.UserUUID))
			require.NoError(t, err)
			require.Equal(t, tc.charge, task.Quota)
			var stored map[string]json.RawMessage
			require.NoError(t, json.Unmarshal([]byte(task.RequestBody), &stored))
			require.JSONEq(t, `5`, string(stored["duration"]), "the provider is asked for the billed 5 s")
			for _, alias := range []string{"seconds", "duration_seconds", "model"} {
				require.NotContains(t, stored, alias)
			}

			muapiRunWorkerStep(t)
			require.EqualValues(t, 1, server.creates.Load())
			require.False(t, server.badBody.Load(), "the provider received duration 5 and no model")
		})
	}
}
