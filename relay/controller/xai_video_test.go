package controller

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/common/logger"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/channeltype"
)

// xaiVideoSetup resets physical balances and enables real SQLite task/log
// persistence; cleanup drains billing before restoring shared configuration.
func xaiVideoSetup(t *testing.T, balance int64, unlimited bool) {
	t.Helper()
	billingAccountingSetup(t, balance)
	// Earlier suites may restore a larger pool after using this shared SQLite
	// fixture. Serialize its connections so post-response billing and request-cost
	// writes queue rather than fail with SQLITE_LOCKED (busy_timeout does not
	// retry shared-cache table locks). The HTTP and billing goroutines still run.
	sqlDB, err := model.DB.DB()
	require.NoError(t, err)
	previousMaxOpen := sqlDB.Stats().MaxOpenConnections
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() {
		drainCriticalTasks(t)
		sqlDB.SetMaxOpenConns(previousMaxOpen)
	})
	previousBatch := config.BatchUpdateEnabled
	config.BatchUpdateEnabled = false
	t.Cleanup(func() { config.BatchUpdateEnabled = previousBatch })
	config.SetLogConsumeEnabled(true)
	require.NoError(t, model.DB.AutoMigrate(&model.AsyncTaskBinding{}))
	require.NoError(t, model.LOG_DB.AutoMigrate(&model.Log{}))
	require.NoError(t, model.DB.Model(&model.Token{}).Where("id = ?", fallbackTokenID).Updates(map[string]any{"remain_quota": balance, "used_quota": 0, "unlimited_quota": unlimited}).Error)
	t.Cleanup(func() { drainCriticalTasks(t) })
}

// xaiVideoContext creates one request using real model mapping and channel price
// resolution. It returns the writer and bounded request ID for ledger assertions.
func xaiVideoContext(t *testing.T, method, path, body, base string, balance int64, group float64, unlimited bool, override *model.VideoPricingLocal) (*gin.Context, *httptest.ResponseRecorder, string) {
	t.Helper()
	channel := &model.Channel{Id: fallbackChannelID, Type: channeltype.XAI, Name: "xai-video-test", Key: "xai-fixture-key"}
	if override != nil {
		require.NoError(t, channel.SetModelPriceConfigs(map[string]model.ModelConfigLocal{"grok-imagine-video-1.5": {Video: override}}))
	}
	hash := sha256.Sum256([]byte(t.Name() + method + path))
	requestID := fmt.Sprintf("xv-%x", hash[:10])
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	gmw.SetLogger(c, logger.Logger)
	c.Request = httptest.NewRequest(method, path, strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.Header.Set("Authorization", "Bearer xai-fixture-key")
	for key, value := range map[string]any{
		ctxkey.Channel: channeltype.XAI, ctxkey.ChannelId: fallbackChannelID, ctxkey.ChannelModel: channel,
		ctxkey.TokenId: fallbackTokenID, ctxkey.TokenName: "fallback-token", ctxkey.Id: fallbackUserID,
		ctxkey.Group: "default", ctxkey.ModelMapping: map[string]string{"alias": "grok-imagine-video-1.5"}, ctxkey.ChannelRatio: group,
		ctxkey.RequestModel: "alias", ctxkey.BaseURL: base, ctxkey.ContentType: "application/json", ctxkey.RequestId: requestID,
		ctxkey.Username: "response-fallback", ctxkey.UserObj: &model.User{Id: fallbackUserID, Quota: balance},
		ctxkey.Config: model.ChannelConfig{}, ctxkey.TokenQuotaUnlimited: unlimited, ctxkey.TokenQuota: balance,
	} {
		c.Set(key, value)
	}
	return c, w, requestID
}

// xaiVideoObservation captures the provider wire request and balances at dispatch
// so assertions never call testing.Fatal from the HTTP server goroutine.
type xaiVideoObservation struct {
	Path, Authorization string
	Body                map[string]any
	UserQuota           int64
	Err                 error
}

// TestXAIVideoBillingHTTP verifies actual HTTP forwarding, native task binding,
// duration/resolution/image prices, overrides, refunds, and free repeated polls.
func TestXAIVideoBillingHTTP(t *testing.T) {
	for _, tc := range []struct {
		name, fields                 string
		status                       int
		charge                       int64
		group                        float64
		unlimited, trusted, override bool
	}{
		{"default_480p", `"duration":5`, 200, 200000, 1, false, false, false},
		{"720p", `"duration":5,"resolution":"720p"`, 200, 350000, 1, false, false, false},
		{"1080p", `"duration":5,"resolution":"1080p"`, 200, 625000, 1, false, false, false},
		{"seconds_alias", `"seconds":5,"resolution":"720p"`, 202, 350000, 1, false, false, false},
		{"duration_seconds_alias", `"duration_seconds":5`, 200, 200000, 1, false, false, false},
		{"image_fee", `"duration":5,"image":{"url":"https://example.com/a.png"}`, 200, 205000, 1, false, false, false},
		{"frame_reference_fees", `"duration":5,"image":{"url":"https://example.com/a.png"},"last_frame":{"url":"https://example.com/b.png"},"reference_images":[{"url":"https://example.com/c.png"}],"reference_audios":[{"voice_id":"eve"}]`, 200, 215000, 1, false, false, false},
		{"group_rate", `"duration":5`, 200, 400000, 2, false, false, false},
		{"free_group", `"duration":5`, 200, 0, 0, false, false, false},
		{"unlimited_token", `"duration":5`, 200, 200000, 1, true, false, false},
		{"trusted_balance", `"duration":5`, 200, 200000, 1, false, true, false},
		{"override", `"duration":5,"resolution":"720p","image":{"url":"https://example.com/a.png"}`, 200, 510000, 1, false, false, true},
		{"upstream_403", `"duration":5`, 403, 0, 1, false, false, false},
		{"upstream_429", `"duration":5`, 429, 0, 1, false, false, false},
		{"upstream_503", `"duration":5`, 503, 0, 1, false, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			balance := int64(10000000)
			if tc.trusted {
				balance = 100000000
			}
			xaiVideoSetup(t, balance, tc.unlimited)
			observations := make(chan xaiVideoObservation, 1)
			var creates, polls atomic.Int32
			jobID := "job-" + tc.name
			upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.Method == http.MethodGet {
					polls.Add(1)
					_, _ = io.WriteString(w, `{"status":"done","video":{"url":"https://example.com/video.mp4","duration":5}}`)
					return
				}
				creates.Add(1)
				var body map[string]any
				err := json.NewDecoder(r.Body).Decode(&body)
				var user model.User
				if err == nil {
					err = model.DB.First(&user, fallbackUserID).Error
				}
				observations <- xaiVideoObservation{r.URL.RequestURI(), r.Header.Get("Authorization"), body, user.Quota, err}
				w.WriteHeader(tc.status)
				if tc.status >= 400 {
					_, _ = io.WriteString(w, `{"error":"provider refused"}`)
					return
				}
				_, _ = io.WriteString(w, `{"request_id":"`+jobID+`"}`)
			}))
			defer upstream.Close()
			previousClient := client.HTTPClient
			client.HTTPClient = upstream.Client()
			defer func() { client.HTTPClient = previousClient }()
			var override *model.VideoPricingLocal
			if tc.override {
				override = &model.VideoPricingLocal{PerSecondUsd: 0.10, InputImageUsd: 0.02, BaseResolution: "480p", ResolutionMultipliers: map[string]float64{"720p": 2}}
			}
			path := "/v1/videos/generations"
			if tc.name == "seconds_alias" {
				path = "/v1/videos"
			}
			body := `{"model":"alias","prompt":"A paper boat.","generate_audio":false,` + tc.fields + `}`
			c, w, requestID := xaiVideoContext(t, http.MethodPost, path, body, upstream.URL+"/v1", balance, tc.group, tc.unlimited, override)
			apiErr := RelayVideoHelper(c)
			if tc.status >= 400 {
				require.NotNil(t, apiErr)
				require.Equal(t, tc.status, apiErr.StatusCode)
			} else {
				require.Nil(t, apiErr)
			}
			require.EqualValues(t, 1, creates.Load())
			observed := <-observations
			require.NoError(t, observed.Err)
			require.Equal(t, "/v1/videos/generations", observed.Path)
			require.Equal(t, "Bearer xai-fixture-key", observed.Authorization)
			require.Equal(t, "grok-imagine-video-1.5", observed.Body["model"])
			require.Equal(t, float64(5), observed.Body["duration"])
			require.Equal(t, false, observed.Body["generate_audio"])
			require.NotContains(t, observed.Body, "seconds")
			require.Equal(t, balance-c.GetInt64(ctxkey.PreConsumedQuotaAmount), observed.UserQuota)
			drainCriticalTasks(t)
			require.Equal(t, balance-tc.charge, reloadUserQuota(t))
			var token model.Token
			require.NoError(t, model.DB.First(&token, fallbackTokenID).Error)
			if tc.unlimited {
				require.Equal(t, balance, token.RemainQuota)
			} else {
				require.Equal(t, balance-tc.charge, token.RemainQuota)
				require.Equal(t, tc.charge, token.UsedQuota)
			}
			if tc.status >= 400 {
				return
			}
			require.Equal(t, tc.charge, requestCostQuota(t, requestID))
			require.JSONEq(t, `{"request_id":"`+jobID+`"}`, w.Body.String())
			binding, err := model.GetAsyncTaskBindingByTaskID(context.Background(), jobID)
			require.NoError(t, err)
			require.Equal(t, fallbackUserID, binding.UserID)
			require.Equal(t, fallbackChannelID, binding.ChannelID)
			require.Equal(t, channeltype.XAI, binding.ChannelType)
			require.Equal(t, "alias", binding.OriginModel)
			require.Equal(t, "grok-imagine-video-1.5", binding.ActualModel)
			var snapshot map[string]any
			require.NoError(t, json.Unmarshal([]byte(binding.RequestParams), &snapshot))
			require.Equal(t, observed.Body["duration"], snapshot["duration_seconds"])
			require.Equal(t, observed.Body["resolution"], snapshot["resolution"])
			for i := range 3 {
				poll, response, _ := xaiVideoContext(t, http.MethodGet, fmt.Sprintf("/v1/videos/%s?poll=%d", jobID, i), "", upstream.URL, balance, tc.group, tc.unlimited, override)
				require.Nil(t, RelayVideoHelper(poll))
				require.Contains(t, response.Body.String(), `"status":"done"`)
				drainCriticalTasks(t)
				require.Equal(t, balance-tc.charge, reloadUserQuota(t), "polling must never charge generation again")
			}
			require.EqualValues(t, 3, polls.Load())
			require.EqualValues(t, 1, creates.Load())
		})
	}
}

// TestXAIVideoAdmissionRejectsUnpricedWork proves invalid, ambiguous and unfunded
// requests make no physical upstream call and leave the user's balance intact.
func TestXAIVideoAdmissionRejectsUnpricedWork(t *testing.T) {
	for _, tc := range []struct {
		fields  string
		balance int64
	}{
		{`"duration":15,"seconds":1`, 10000000},
		{`"duration":5,"size":"480p","resolution":"1080p"`, 10000000},
		{`"duration":16`, 10000000}, {`"duration":5`, 1},
	} {
		t.Run(tc.fields+fmt.Sprint(tc.balance), func(t *testing.T) {
			xaiVideoSetup(t, tc.balance, false)
			var calls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); w.WriteHeader(500) }))
			defer upstream.Close()
			c, _, _ := xaiVideoContext(t, "POST", "/v1/videos", `{"model":"alias","prompt":"test",`+tc.fields+`}`, upstream.URL, tc.balance, 1, false, nil)
			require.NotNil(t, RelayVideoHelper(c))
			drainCriticalTasks(t)
			require.Zero(t, calls.Load())
			require.Equal(t, tc.balance, reloadUserQuota(t))
		})
	}
}

// xaiDisconnectedWriter simulates a downstream client that has stopped reading.
type xaiDisconnectedWriter struct{ gin.ResponseWriter }

// Write returns the simulated connection error without accepting response bytes.
func (w xaiDisconnectedWriter) Write(body []byte) (int, error) {
	return 0, errors.New("simulated client disconnect")
}

// TestXAIVideoAcceptedDisconnect verifies accepted work is neither refunded nor lost
// from the task store when the caller cannot receive its creation receipt.
func TestXAIVideoAcceptedDisconnect(t *testing.T) {
	const balance int64 = 10000000
	xaiVideoSetup(t, balance, false)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"request_id":"accepted-disconnected-job"}`)
	}))
	defer server.Close()
	previous := client.HTTPClient
	client.HTTPClient = server.Client()
	defer func() { client.HTTPClient = previous }()
	c, _, requestID := xaiVideoContext(t, "POST", "/v1/videos/generations", `{"model":"alias","prompt":"test","duration":5}`, server.URL, balance, 1, false, nil)
	c.Writer = xaiDisconnectedWriter{c.Writer}
	apiErr := RelayVideoHelper(c)
	require.NotNil(t, apiErr)
	drainCriticalTasks(t)
	require.Equal(t, balance-200000, reloadUserQuota(t))
	require.EqualValues(t, 200000, requestCostQuota(t, requestID))
	task, err := model.GetAsyncTaskBindingByTaskID(context.Background(), "accepted-disconnected-job")
	require.NoError(t, err)
	require.Equal(t, fallbackUserID, task.UserID)
}
