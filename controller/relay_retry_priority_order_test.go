package controller

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/common/graceful"
	"github.com/Laisky/one-api/middleware"
	dbmodel "github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/model"
)

// retryOrderGroup and retryOrderModel identify the routing pool shared by every
// scenario in this file.
const (
	retryOrderGroup = "default"
	retryOrderModel = "gpt-4o-mini"
)

// retryOrderChannel builds an enabled channel serving retryOrderModel in
// retryOrderGroup at the given priority.
func retryOrderChannel(id int, name string, priority int64) *dbmodel.Channel {
	return &dbmodel.Channel{
		Id:       id,
		Name:     name,
		Type:     channeltype.OpenAI,
		Status:   dbmodel.ChannelStatusEnabled,
		Models:   retryOrderModel,
		Group:    retryOrderGroup,
		Priority: &priority,
	}
}

// setupRetryOrderDB points the package globals at an isolated in-memory SQLite
// database seeded with channels (and their abilities), then rebuilds the in-memory
// channel cache from it so BOTH routing paths (memory cache and DB) see the same
// pool. Everything is restored on cleanup.
func setupRetryOrderDB(t *testing.T, channels []*dbmodel.Channel) {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	// A single connection keeps every query on the same in-memory database.
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, db.AutoMigrate(&dbmodel.Channel{}, &dbmodel.Ability{}))

	originalDB := dbmodel.DB
	dbmodel.DB = db
	t.Cleanup(func() { dbmodel.DB = originalDB })

	originalSQLite := common.UsingSQLite.Load()
	common.UsingSQLite.Store(true)
	t.Cleanup(func() { common.UsingSQLite.Store(originalSQLite) })

	for _, ch := range channels {
		require.NoError(t, db.Create(ch).Error)
		require.NoError(t, ch.AddAbilities())
	}

	// Rebuild the memory cache from the seeded DB so the cache path routes over
	// the same channels as the DB path.
	dbmodel.InitChannelCache()
	t.Cleanup(dbmodel.InitChannelCache)
}

// retryOrderOutcome scripts the upstream outcome of one attempt on channelID.
// Returning nil means the attempt succeeded.
type retryOrderOutcome func(channelID int) *model.ErrorWithStatusCode

// retryOrderResult captures what a scripted Relay run observed.
type retryOrderResult struct {
	// order is the channel id sequence the relay loop attempted, initial channel
	// first.
	order []int
	// status is the HTTP status written to the client (200 when no error body was
	// written because an attempt succeeded).
	status int
	// body is the client-facing response body, if any.
	body []byte
	// processedErrors counts the async channel-error processings that were
	// dispatched, one per failed attempt.
	processedErrors int
}

// runRelayRetryScenario drives the real controller.Relay retry loop with the
// initial channel bound exactly as the distributor middleware would bind it, using
// scripted upstream outcomes instead of a live adaptor. memoryCache selects the
// routing path under test.
func runRelayRetryScenario(t *testing.T, initial *dbmodel.Channel, outcome retryOrderOutcome, memoryCache bool, retryTimes int) retryOrderResult {
	t.Helper()
	gin.SetMode(gin.TestMode)

	originalRetryTimes := config.RetryTimes
	config.RetryTimes = retryTimes
	t.Cleanup(func() { config.RetryTimes = originalRetryTimes })

	originalMemoryCache := config.MemoryCacheEnabled
	config.MemoryCacheEnabled = memoryCache
	t.Cleanup(func() { config.MemoryCacheEnabled = originalMemoryCache })

	originalDebug := config.DebugEnabled
	config.DebugEnabled = true // exercise the diagnostic branches of the loop too
	t.Cleanup(func() { config.DebugEnabled = originalDebug })

	processed := make(chan struct{}, 64)
	processChannelRelayErrorForTest = func(context.Context, processChannelRelayErrorParams) {
		processed <- struct{}{}
	}
	t.Cleanup(func() {
		drainCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = graceful.Drain(drainCtx)
		processChannelRelayErrorForTest = nil
	})

	var order []int
	relayHelperForTest = func(c *gin.Context, _ int) *model.ErrorWithStatusCode {
		id := c.GetInt(ctxkey.ChannelId)
		order = append(order, id)
		return outcome(id)
	}
	t.Cleanup(func() { relayHelperForTest = nil })

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	body, err := json.Marshal(map[string]any{
		"model":    retryOrderModel,
		"messages": []map[string]string{{"role": "user", "content": "hi"}},
	})
	require.NoError(t, err)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(ctxkey.Id, 1)
	c.Set(ctxkey.TokenId, 2)
	c.Set(ctxkey.Group, retryOrderGroup)
	c.Set(ctxkey.RequestModel, retryOrderModel)
	middleware.SetupContextForSelectedChannel(c, initial, retryOrderModel)

	Relay(c)

	drainCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, graceful.Drain(drainCtx))

	return retryOrderResult{
		order:           order,
		status:          recorder.Code,
		body:            recorder.Body.Bytes(),
		processedErrors: len(processed),
	}
}

// rateLimited returns the scripted 429 outcome upstreams produce when a channel
// is rate limited.
func rateLimited(channelID int) *model.ErrorWithStatusCode {
	return &model.ErrorWithStatusCode{
		StatusCode: http.StatusTooManyRequests,
		Error: model.Error{
			Message: "rate limit exceeded",
			Type:    model.ErrorTypeRateLimit,
			Code:    "rate_limit_exceeded",
		},
	}
}

// serverError returns a scripted upstream 500 outcome.
func serverError(channelID int) *model.ErrorWithStatusCode {
	return &model.ErrorWithStatusCode{
		StatusCode: http.StatusInternalServerError,
		Error: model.Error{
			Message: "upstream exploded",
			Type:    model.ErrorTypeServer,
		},
	}
}

// failOnly builds an outcome that fails with fail on the listed channels and
// succeeds everywhere else.
func failOnly(fail retryOrderOutcome, failing ...int) retryOrderOutcome {
	set := make(map[int]bool, len(failing))
	for _, id := range failing {
		set[id] = true
	}
	return func(channelID int) *model.ErrorWithStatusCode {
		if set[channelID] {
			return fail(channelID)
		}
		return nil
	}
}

// routingPaths enumerates the two channel-selection implementations the retry
// loop can run on; every ordering scenario must hold on both.
var routingPaths = []struct {
	name        string
	memoryCache bool
}{
	{name: "memory-cache", memoryCache: true},
	{name: "database", memoryCache: false},
}

// TestRelayRetry_429StrictPriorityOrder is the reproduce-first guard for the
// retry-ordering bug: with A(priority=10), B(5), C(0) and A returning 429, the
// retry loop used to exclude A and then ask the selector to "skip the highest
// remaining tier" — which, once A is gone, is B — so the first retry went to C and
// B was only reached last (A → C → B). Operators expect a strict priority walk
// (A → B → C). The scenario runs every channel as rate limited so the complete
// walk is observable.
func TestRelayRetry_429StrictPriorityOrder(t *testing.T) {
	for _, path := range routingPaths {
		t.Run(path.name, func(t *testing.T) {
			channelA := retryOrderChannel(1, "A-prio-10", 10)
			channelB := retryOrderChannel(2, "B-prio-5", 5)
			channelC := retryOrderChannel(3, "C-prio-0", 0)
			setupRetryOrderDB(t, []*dbmodel.Channel{channelA, channelB, channelC})

			got := runRelayRetryScenario(t, channelA, rateLimited, path.memoryCache, 3)

			require.Equal(t, []int{1, 2, 3}, got.order,
				"429 retries must walk channels strictly by descending priority")
			require.Equal(t, http.StatusTooManyRequests, got.status)
			require.Contains(t, string(got.body), "All available channels (3)")
			require.Equal(t, 3, got.processedErrors, "every failed attempt must be error-processed")
		})
	}
}

// TestRelayRetry_429NextTierSucceeds pins the common production shape: the top
// channel is rate limited and the very next tier serves the request, so exactly
// one retry happens and it lands on B, not on the lowest tier.
func TestRelayRetry_429NextTierSucceeds(t *testing.T) {
	for _, path := range routingPaths {
		t.Run(path.name, func(t *testing.T) {
			channelA := retryOrderChannel(1, "A-prio-10", 10)
			channelB := retryOrderChannel(2, "B-prio-5", 5)
			channelC := retryOrderChannel(3, "C-prio-0", 0)
			setupRetryOrderDB(t, []*dbmodel.Channel{channelA, channelB, channelC})

			got := runRelayRetryScenario(t, channelA, failOnly(rateLimited, 1), path.memoryCache, 3)

			require.Equal(t, []int{1, 2}, got.order,
				"the first retry after a 429 on the top tier must land on the next tier")
			require.Equal(t, http.StatusOK, got.status, "a successful retry must not write an error body")
			require.Empty(t, got.body)
			require.Equal(t, 1, got.processedErrors)
		})
	}
}

// TestRelayRetry_429SameTierSiblingBeforeLowerTier pins strict priority order
// when the rate-limited channel has a same-priority sibling: the sibling shares
// the operator's preference, so it is tried before dropping a tier. (A1 → A2 → B)
func TestRelayRetry_429SameTierSiblingBeforeLowerTier(t *testing.T) {
	for _, path := range routingPaths {
		t.Run(path.name, func(t *testing.T) {
			channelA1 := retryOrderChannel(1, "A1-prio-10", 10)
			channelA2 := retryOrderChannel(2, "A2-prio-10", 10)
			channelB := retryOrderChannel(3, "B-prio-5", 5)
			setupRetryOrderDB(t, []*dbmodel.Channel{channelA1, channelA2, channelB})

			got := runRelayRetryScenario(t, channelA1, failOnly(rateLimited, 1, 2), path.memoryCache, 3)

			require.Equal(t, []int{1, 2, 3}, got.order,
				"a same-priority sibling must be tried before any lower tier")
			require.Equal(t, http.StatusOK, got.status)
		})
	}
}

// TestRelayRetry_5xxStrictPriorityOrder guards backward compatibility: the
// non-429 retry path already walked channels by descending priority and must keep
// doing so after the 429 fix.
func TestRelayRetry_5xxStrictPriorityOrder(t *testing.T) {
	for _, path := range routingPaths {
		t.Run(path.name, func(t *testing.T) {
			channelA := retryOrderChannel(1, "A-prio-10", 10)
			channelB := retryOrderChannel(2, "B-prio-5", 5)
			channelC := retryOrderChannel(3, "C-prio-0", 0)
			setupRetryOrderDB(t, []*dbmodel.Channel{channelA, channelB, channelC})

			got := runRelayRetryScenario(t, channelA, serverError, path.memoryCache, 3)

			require.Equal(t, []int{1, 2, 3}, got.order,
				"5xx retries must walk channels strictly by descending priority")
			require.Equal(t, http.StatusInternalServerError, got.status)
			require.Equal(t, 3, got.processedErrors)
		})
	}
}

// TestSelectRetryChannel_WalksTiersAfterExclusion drives selectRetryChannel the
// way the retry loop does — mark each returned channel failed, select again —
// and pins the resulting walk on both routing paths. It also pins that running
// out of candidates yields an error the loop classifies as expected exhaustion
// (WARN), not an infrastructure failure (ERROR).
func TestSelectRetryChannel_WalksTiersAfterExclusion(t *testing.T) {
	cases := []struct {
		name     string
		channels []*dbmodel.Channel
		initial  int
		wantWalk []int
	}{
		{
			name: "one channel per tier walks A B C",
			channels: []*dbmodel.Channel{
				retryOrderChannel(1, "A-prio-10", 10),
				retryOrderChannel(2, "B-prio-5", 5),
				retryOrderChannel(3, "C-prio-0", 0),
			},
			initial:  1,
			wantWalk: []int{2, 3},
		},
		{
			name: "same-tier sibling before lower tier",
			channels: []*dbmodel.Channel{
				retryOrderChannel(1, "A1-prio-10", 10),
				retryOrderChannel(2, "A2-prio-10", 10),
				retryOrderChannel(3, "B-prio-5", 5),
			},
			initial:  1,
			wantWalk: []int{2, 3},
		},
		{
			name: "initial failure below the top tier still walks downward only",
			channels: []*dbmodel.Channel{
				retryOrderChannel(1, "A-prio-10", 10),
				retryOrderChannel(2, "B-prio-5", 5),
				retryOrderChannel(3, "C-prio-0", 0),
			},
			// The distributor only lands on B when A is unavailable; the retry
			// walk must then continue with the best untried channel, which is A
			// (still the highest remaining tier), then C.
			initial:  2,
			wantWalk: []int{1, 3},
		},
	}

	for _, path := range routingPaths {
		for _, tc := range cases {
			t.Run(path.name+"/"+tc.name, func(t *testing.T) {
				originalMemoryCache := config.MemoryCacheEnabled
				config.MemoryCacheEnabled = path.memoryCache
				t.Cleanup(func() { config.MemoryCacheEnabled = originalMemoryCache })
				setupRetryOrderDB(t, tc.channels)

				failed := map[int]bool{tc.initial: true}
				var walk []int
				for range len(tc.channels) + 1 {
					channel, err := selectRetryChannel(context.Background(), retryOrderGroup, retryOrderModel, failed, retrySelectStrictPriority)
					if err != nil {
						require.True(t, isExpectedChannelSelectionExhaustedError(err),
							"exhaustion must stay classified as expected: %+v", err)
						break
					}
					walk = append(walk, channel.Id)
					failed[channel.Id] = true
				}
				require.Equal(t, tc.wantWalk, walk)
				require.Len(t, failed, len(tc.channels), "the walk must visit every channel exactly once")
			})
		}
	}
}

// TestRelayRetry_413PrefersLargerMaxTokens guards backward compatibility of the
// 413 policy, which is independent of the 429 fix: after A(4096 max_tokens)
// rejects the request as too large, B shares that max_tokens and is skipped in
// favour of C(8192) even though B outranks C. Only the memory-cache selector
// implements the max_tokens filter, so the scenario is pinned on that path.
func TestRelayRetry_413PrefersLargerMaxTokens(t *testing.T) {
	channelA := retryOrderChannel(1, "A-prio-10-4k", 10)
	channelA.ModelConfigs = stringPtr(`{"gpt-4o-mini": {"max_tokens": 4096}}`)
	channelB := retryOrderChannel(2, "B-prio-5-4k", 5)
	channelB.ModelConfigs = stringPtr(`{"gpt-4o-mini": {"max_tokens": 4096}}`)
	channelC := retryOrderChannel(3, "C-prio-0-8k", 0)
	channelC.ModelConfigs = stringPtr(`{"gpt-4o-mini": {"max_tokens": 8192}}`)
	setupRetryOrderDB(t, []*dbmodel.Channel{channelA, channelB, channelC})

	tooLarge := func(int) *model.ErrorWithStatusCode {
		return &model.ErrorWithStatusCode{
			StatusCode: http.StatusRequestEntityTooLarge,
			Error:      model.Error{Message: "request too large", Type: model.ErrorTypeUpstream},
		}
	}
	got := runRelayRetryScenario(t, channelA, failOnly(tooLarge, 1, 2), true, 3)

	require.Equal(t, []int{1, 3}, got.order,
		"413 retries must skip channels whose max_tokens already rejected the request")
	require.Equal(t, http.StatusOK, got.status)
}
