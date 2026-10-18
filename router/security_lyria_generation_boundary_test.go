package router

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/billing/ratio"
	"github.com/Laisky/one-api/relay/channeltype"
)

const (
	lyriaGenerationBoundaryModel = "google/lyria-3-clip-preview"
	lyriaGenerationBoundaryQuota = int64(20000)
	lyriaGenerationFixtureAudio  = "UklGRiYAAABXQVZFZm10IBAAAAABAAEAQB8AAIA+AAACABAAZGF0YQIAAAAAAA=="
)

// lyriaGenerationObservation records a synthetic provider request and the
// physical user reservation before its response. It carries fixture errors.
type lyriaGenerationObservation struct {
	body       map[string]json.RawMessage
	path       string
	userQuota  int64
	tokenQuota int64
	err        error
}

// lyriaGenerationCapture retains loopback provider observations and response
// write errors behind a mutex so assertions do not race the HTTP handler.
type lyriaGenerationCapture struct {
	mu          sync.Mutex
	requests    []lyriaGenerationObservation
	writeErrors []error
}

// snapshot returns copies of the observed requests and response write errors.
// Parameters: none. Returns: immutable slice snapshots for test assertions.
func (p *lyriaGenerationCapture) snapshot() ([]lyriaGenerationObservation, []error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]lyriaGenerationObservation(nil), p.requests...), append([]error(nil), p.writeErrors...)
}

// newLyriaGenerationBoundaryRouter builds the shipped router with real token
// auth, OpenRouter conversion and SQLite billing. Parameters: t owns cleanup.
// Returns: the router, synthetic token and TLS loopback provider capture.
func newLyriaGenerationBoundaryRouter(t *testing.T) (*gin.Engine, string, *lyriaGenerationCapture) {
	t.Helper()
	capture := &lyriaGenerationCapture{}
	upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		observed := lyriaGenerationObservation{path: r.URL.Path}
		observed.err = json.NewDecoder(r.Body).Decode(&observed.body)
		if observed.err == nil {
			var user model.User
			observed.err = model.DB.Where("id = ?", passthroughKeysUserID).Take(&user).Error
			observed.userQuota = user.Quota
		}
		if observed.err == nil {
			var token model.Token
			observed.err = model.DB.Where("id = ?", passthroughKeysTokenID).Take(&token).Error
			observed.tokenQuota = token.RemainQuota
		}
		capture.mu.Lock()
		capture.requests = append(capture.requests, observed)
		capture.mu.Unlock()
		if observed.err != nil {
			http.Error(w, "fixture request failed", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		audioField := ""
		if string(observed.body["model"]) == `"`+lyriaGenerationBoundaryModel+`"` {
			audioField = `,"audio":{"id":"generation-boundary-audio","data":"` + lyriaGenerationFixtureAudio + `","transcript":"Music"}`
		}
		reply := `{"id":"generation-boundary-fixture","object":"chat.completion","model":` + string(observed.body["model"]) + `,"choices":[{"index":0,"finish_reason":"stop","message":{"role":"assistant","content":"Generated music"` + audioField + `}}],"usage":{"prompt_tokens":10,"completion_tokens":20,"total_tokens":30}}`
		if _, err := io.WriteString(w, reply); err != nil {
			capture.mu.Lock()
			capture.writeErrors = append(capture.writeErrors, err)
			capture.mu.Unlock()
		}
	}))
	t.Cleanup(upstream.Close)
	oldBatch, oldPreconsume := config.BatchUpdateEnabled, config.PreConsumedQuota
	oldGroups := ratio.GroupRatio2JSONString()
	config.BatchUpdateEnabled, config.PreConsumedQuota = false, 500
	require.NoError(t, ratio.UpdateGroupRatioByJSONString(`{"default":1}`))
	t.Cleanup(func() {
		config.BatchUpdateEnabled, config.PreConsumedQuota = oldBatch, oldPreconsume
		require.NoError(t, ratio.UpdateGroupRatioByJSONString(oldGroups))
	})
	engine, key := ambiguousKeysRouter(t, upstream.URL, channeltype.OpenRouter, "alias", true)
	require.NoError(t, model.DB.Model(&model.Channel{}).Where("id = ?", 929293).
		Update("model_mapping", `{"alias":"`+lyriaGenerationBoundaryModel+`"}`).Error)
	client.HTTPClient = upstream.Client()
	return engine, key, capture
}

// lyriaGenerationLedger captures all physical balances and generation receipts
// in the isolated database, including finite-token use and request-cost rows.
type lyriaGenerationLedger struct {
	user  model.User
	token model.Token
	costs []model.UserRequestCost
	logs  []model.Log
}

// readLyriaGenerationLedger reads the isolated generation billing records.
// Parameters: t owns database assertions. Returns: the current ledger snapshot.
func readLyriaGenerationLedger(t *testing.T) lyriaGenerationLedger {
	t.Helper()
	var ledger lyriaGenerationLedger
	require.NoError(t, model.DB.Where("id = ?", passthroughKeysUserID).Take(&ledger.user).Error)
	require.NoError(t, model.DB.Where("id = ?", passthroughKeysTokenID).Take(&ledger.token).Error)
	require.NoError(t, model.DB.Where("user_id = ?", passthroughKeysUserID).Find(&ledger.costs).Error)
	require.NoError(t, model.LOG_DB.Where("user_id = ?", passthroughKeysUserID).Order("id").Find(&ledger.logs).Error)
	return ledger
}

// postLyriaGenerationBoundary sends a request through the shipped public route.
// Parameters: t owns billing drain, engine and key authenticate the request,
// body supplies valid JSON independently of query parameters.
// Returns: the completed response recorder after detached billing drains.
func postLyriaGenerationBoundary(t *testing.T, engine *gin.Engine, key, body string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer sk-"+key)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	engine.ServeHTTP(response, request)
	drainCriticalTasksForRouter(t)
	return response
}

// requireLyriaGenerationRejected proves an invalid request never starts provider
// work or mutates any billing record. Parameters: t owns assertions, response is
// the route result, capture observes provider work, and before is the ledger.
// Returns: none; the public route must reject without charging a generation.
func requireLyriaGenerationRejected(t *testing.T, response *httptest.ResponseRecorder, capture *lyriaGenerationCapture, before lyriaGenerationLedger) {
	t.Helper()
	after := readLyriaGenerationLedger(t)
	requests, writeErrors := capture.snapshot()
	require.Empty(t, writeErrors)
	var forwardedCounts []string
	for _, request := range requests {
		forwardedCounts = append(forwardedCounts, string(request.body["n"]))
	}
	var costQuota, logQuota int64
	for _, cost := range after.costs {
		costQuota += cost.Quota
	}
	for _, log := range after.logs {
		logQuota += int64(log.Quota)
	}
	t.Logf("HTTP %d; provider calls=%d; outbound n=%v; user debit=%d; finite-token debit=%d; request-cost quota=%d (%d rows); consume-log quota=%d (%d rows)",
		response.Code, len(requests), forwardedCounts, before.user.Quota-after.user.Quota, before.token.RemainQuota-after.token.RemainQuota,
		costQuota, len(after.costs), logQuota, len(after.logs))
	require.Zero(t, len(requests), "an invalid generation request reached the provider (HTTP %d)", response.Code)
	require.Equal(t, before.user.Quota, after.user.Quota, "rejection must not debit the user")
	require.Equal(t, before.user.UsedQuota, after.user.UsedQuota)
	require.Equal(t, before.token.RemainQuota, after.token.RemainQuota, "rejection must not debit the finite token")
	require.Equal(t, before.token.UsedQuota, after.token.UsedQuota)
	require.Equal(t, before.costs, after.costs, "rejection must not create or alter a request-cost receipt")
	require.Equal(t, before.logs, after.logs, "rejection must not create a consume-log receipt")
	require.Equal(t, http.StatusBadRequest, response.Code, response.Body.String())
	var rejection struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &rejection))
	require.Equal(t, "unbounded_generation_request", rejection.Error.Code, "the valid JSON must reach generation admission")
}

// TestSecurityLyriaGenerationJSONBoundary retains two generation-specific
// authenticated-router controls. Parameters: t owns isolated fixtures. Returns:
// none; neither invalid count may dispatch or alter balances or receipt rows.
func TestSecurityLyriaGenerationJSONBoundary(t *testing.T) {
	for _, tc := range []struct {
		name, fields string
	}{
		{"canonical_multiple", `"n":2`},
		{"extra_body_multiple", `"extra_body":{"n":2}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			engine, key, capture := newLyriaGenerationBoundaryRouter(t)
			before := readLyriaGenerationLedger(t)
			body := `{"model":"alias","messages":[{"role":"user","content":"Create music"}],"max_tokens":128,` + tc.fields + `}`
			response := postLyriaGenerationBoundary(t, engine, key, body)
			requireLyriaGenerationRejected(t, response, capture, before)
		})
	}
}

// TestSecurityLyriaGenerationSingleBoundary retains ordinary single-generation
// requests and proves their one dispatch reserves and settles one catalog charge.
// Parameters: t owns synthetic fixtures. Returns: none; the user, finite token,
// request-cost row and consume log must agree on exactly 20,000 quota units.
func TestSecurityLyriaGenerationSingleBoundary(t *testing.T) {
	for _, tc := range []struct {
		name, fields string
	}{
		{name: "omitted_count"},
		{name: "canonical_single", fields: `,"n":1`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			engine, key, capture := newLyriaGenerationBoundaryRouter(t)
			before := readLyriaGenerationLedger(t)
			require.False(t, before.token.UnlimitedQuota, "this fixture must exercise a finite token")
			body := `{"model":"alias","messages":[{"role":"user","content":"Create music"}],"max_tokens":128` + tc.fields + `}`
			response := postLyriaGenerationBoundary(t, engine, key, body)
			require.Equal(t, http.StatusOK, response.Code, response.Body.String())
			requests, writeErrors := capture.snapshot()
			require.Empty(t, writeErrors)
			require.Len(t, requests, 1, "one generation must cause exactly one provider dispatch")
			require.NoError(t, requests[0].err)
			require.Equal(t, "/v1/chat/completions", requests[0].path)
			require.Equal(t, `"`+lyriaGenerationBoundaryModel+`"`, string(requests[0].body["model"]))
			var reply struct {
				Choices []struct {
					Message struct {
						Audio *struct {
							Data string `json:"data"`
						} `json:"audio"`
					} `json:"message"`
				} `json:"choices"`
			}
			require.NoError(t, json.Unmarshal(response.Body.Bytes(), &reply))
			require.Len(t, reply.Choices, 1)
			require.NotNil(t, reply.Choices[0].Message.Audio)
			require.Equal(t, lyriaGenerationFixtureAudio, reply.Choices[0].Message.Audio.Data)
			if tc.fields != "" {
				require.Equal(t, "1", string(requests[0].body["n"]))
			}
			require.Equal(t, before.user.Quota-lyriaGenerationBoundaryQuota, requests[0].userQuota, "the generation must be reserved before provider work")
			require.Equal(t, before.token.RemainQuota-lyriaGenerationBoundaryQuota, requests[0].tokenQuota, "the finite token must be reserved before provider work")
			after := readLyriaGenerationLedger(t)
			require.Equal(t, before.user.Quota-lyriaGenerationBoundaryQuota, after.user.Quota)
			require.Equal(t, before.user.UsedQuota+lyriaGenerationBoundaryQuota, after.user.UsedQuota)
			require.Equal(t, before.token.RemainQuota-lyriaGenerationBoundaryQuota, after.token.RemainQuota)
			require.Equal(t, before.token.UsedQuota+lyriaGenerationBoundaryQuota, after.token.UsedQuota)
			require.Len(t, after.costs, 1)
			require.Equal(t, lyriaGenerationBoundaryQuota, after.costs[0].Quota)
			require.Len(t, after.logs, 1)
			require.Equal(t, model.LogTypeConsume, after.logs[0].Type)
			require.EqualValues(t, lyriaGenerationBoundaryQuota, after.logs[0].Quota)
			require.Equal(t, lyriaGenerationBoundaryModel, after.logs[0].ModelName)
			require.Equal(t, after.costs[0].RequestID, after.logs[0].RequestId, "the cost and consume log must describe the same generation")
		})
	}
}

// TestSecurityLyriaGenerationTokenPricedMultipleBoundary retains the ordinary
// token-priced chat contract that permits multiple choices. Parameters: t owns
// synthetic fixtures. Returns: none; one dispatched n=2 request must retain
// consistent finite-token, user, request-cost and consume-log billing.
func TestSecurityLyriaGenerationTokenPricedMultipleBoundary(t *testing.T) {
	engine, key, capture := newLyriaGenerationBoundaryRouter(t)
	const tokenModel = "gpt-4o-mini"
	require.NoError(t, model.DB.Model(&model.Channel{}).Where("id = ?", 929293).
		Update("model_mapping", `{"alias":"`+tokenModel+`"}`).Error)
	channel := model.Channel{Id: 929293}
	require.NoError(t, channel.SetModelPriceConfigs(map[string]model.ModelConfigLocal{
		tokenModel: {Ratio: 1, CompletionRatio: 2},
	}))
	require.NoError(t, model.DB.Model(&model.Channel{}).Where("id = ?", channel.Id).Update("model_configs", channel.ModelConfigs).Error)
	before := readLyriaGenerationLedger(t)
	require.False(t, before.token.UnlimitedQuota)
	body := `{"model":"alias","messages":[{"role":"user","content":"Hello"}],"max_tokens":128,"n":2}`
	response := postLyriaGenerationBoundary(t, engine, key, body)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	requests, writeErrors := capture.snapshot()
	require.Empty(t, writeErrors)
	require.Len(t, requests, 1)
	require.NoError(t, requests[0].err)
	require.Equal(t, "/v1/chat/completions", requests[0].path)
	require.Equal(t, `"`+tokenModel+`"`, string(requests[0].body["model"]))
	// The fixed prompt estimates eight tokens; the 500-token input buffer and
	// max_tokens=128 reserve (500+8)*1+128*2=764 quota before provider work.
	const reservation = int64(764)
	require.Equal(t, before.user.Quota-reservation, requests[0].userQuota)
	require.Equal(t, before.token.RemainQuota-reservation, requests[0].tokenQuota)
	require.Equal(t, "2", string(requests[0].body["n"]))
	after := readLyriaGenerationLedger(t)
	require.Len(t, after.costs, 1)
	require.Len(t, after.logs, 1)
	require.Equal(t, model.LogTypeConsume, after.logs[0].Type)
	// Provider usage is aggregate across choices: 10*1+20*2=50 quota.
	// This must reject a mistaken generation charge or an extra n multiplier.
	const charge = int64(50)
	require.Equal(t, charge, after.costs[0].Quota)
	require.Equal(t, before.user.Quota-charge, after.user.Quota)
	require.Equal(t, before.user.UsedQuota+charge, after.user.UsedQuota)
	require.Equal(t, before.token.RemainQuota-charge, after.token.RemainQuota)
	require.Equal(t, before.token.UsedQuota+charge, after.token.UsedQuota)
	require.EqualValues(t, charge, after.logs[0].Quota)
	require.Equal(t, tokenModel, after.logs[0].ModelName)
	require.Equal(t, after.costs[0].RequestID, after.logs[0].RequestId)
}
