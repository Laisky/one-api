package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/common/logger"
	"github.com/Laisky/one-api/model"
)

// TestLowBalanceRelayRateLimit verifies that the low-balance relay limiter only
// throttles non-privileged users whose balance is below the configured threshold,
// while leaving everyone else (and the default configuration) untouched.
//
// Subtests run sequentially (no t.Parallel) because they mutate shared config
// globals; each restores any value it changes.
func TestLowBalanceRelayRateLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Preserve and restore config globals mutated by this test.
	orig := struct {
		num         int
		dur         int64
		global      int
		threshold   float64
		perUnit     float64
		rateDisable bool
	}{
		num:         config.LowBalanceRelayRateLimitNum,
		dur:         config.LowBalanceRelayRateLimitDuration,
		global:      config.GlobalRelayRateLimitNum,
		threshold:   config.LowBalanceThreshold,
		perUnit:     config.QuotaPerUnit,
		rateDisable: config.RateLimitDisabled,
	}
	defer func() {
		config.LowBalanceRelayRateLimitNum = orig.num
		config.LowBalanceRelayRateLimitDuration = orig.dur
		config.GlobalRelayRateLimitNum = orig.global
		config.LowBalanceThreshold = orig.threshold
		config.QuotaPerUnit = orig.perUnit
		config.RateLimitDisabled = orig.rateDisable
	}()

	// Force the in-memory limiter path: redisEnabled defaults to true but RDB is
	// nil in unit tests (InitRedisClient never runs here). With Redis disabled the
	// middleware reads balances via CacheGetUserQuota -> GetUserQuota -> DB, so stand
	// up an in-memory SQLite DB seeded with the test users.
	origRedis := common.IsRedisEnabled()
	common.SetRedisEnabled(false)
	defer common.SetRedisEnabled(origRedis)

	origDB := model.DB
	db, err := gorm.Open(sqlite.Open("file:lowbalancetest?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.User{}))
	model.DB = db
	defer func() { model.DB = origDB }()

	// access_token and aff_code carry unique indexes, so give each row distinct values.
	seedUsers := []*model.User{
		{Id: 90001, Username: "lb-zero", Password: "placeholder", AccessToken: "lb-token-90001", AffCode: "lb-aff-90001", Role: model.RoleCommonUser, Status: model.UserStatusEnabled, Quota: 0},
		{Id: 90002, Username: "lb-rich", Password: "placeholder", AccessToken: "lb-token-90002", AffCode: "lb-aff-90002", Role: model.RoleCommonUser, Status: model.UserStatusEnabled, Quota: 300000},
		{Id: 90003, Username: "lb-admin", Password: "placeholder", AccessToken: "lb-token-90003", AffCode: "lb-aff-90003", Role: model.RoleAdminUser, Status: model.UserStatusEnabled, Quota: 0},
		{Id: 90004, Username: "lb-noop", Password: "placeholder", AccessToken: "lb-token-90004", AffCode: "lb-aff-90004", Role: model.RoleCommonUser, Status: model.UserStatusEnabled, Quota: 0},
		{Id: 90005, Username: "lb-debug", Password: "placeholder", AccessToken: "lb-token-90005", AffCode: "lb-aff-90005", Role: model.RoleCommonUser, Status: model.UserStatusEnabled, Quota: 0},
		{Id: 90006, Username: "lb-dur", Password: "placeholder", AccessToken: "lb-token-90006", AffCode: "lb-aff-90006", Role: model.RoleCommonUser, Status: model.UserStatusEnabled, Quota: 0},
	}
	for _, u := range seedUsers {
		require.NoError(t, db.Create(u).Error)
	}

	// Stricter-than-global config so the limiter engages.
	config.RateLimitDisabled = false
	config.GlobalRelayRateLimitNum = 480
	config.LowBalanceRelayRateLimitNum = 2
	config.LowBalanceRelayRateLimitDuration = 3600 // long window keeps the limit deterministic
	config.LowBalanceThreshold = 0.5
	config.QuotaPerUnit = 500000 // $0.5 == 250000 quota units

	// run invokes the middleware once for the given user and reports the outcome.
	run := func(user *model.User) (status int, body string, aborted bool) {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
		gmw.SetLogger(c, logger.Logger)
		if user != nil {
			c.Set(ctxkey.UserObj, user)
		}
		LowBalanceRelayRateLimit()(c)
		return rec.Code, rec.Body.String(), c.IsAborted()
	}

	t.Run("throttles_low_balance_user_after_limit", func(t *testing.T) {
		user := &model.User{Id: 90001, Role: model.RoleCommonUser, Quota: 0}
		// First maxRequestNum requests are allowed.
		for i := 0; i < 2; i++ {
			code, _, aborted := run(user)
			assert.Equal(t, http.StatusOK, code)
			assert.False(t, aborted)
		}
		// The next one is throttled with a clear low-balance explanation.
		code, body, aborted := run(user)
		assert.Equal(t, http.StatusTooManyRequests, code)
		assert.True(t, aborted)
		assert.Contains(t, body, "balance")
		assert.Contains(t, body, "top up")
		assert.Contains(t, body, "0.50")
	})

	t.Run("sufficient_balance_not_throttled", func(t *testing.T) {
		// $0.60 balance is above the $0.50 threshold.
		user := &model.User{Id: 90002, Role: model.RoleCommonUser, Quota: 300000}
		for i := 0; i < 5; i++ {
			code, _, aborted := run(user)
			assert.Equal(t, http.StatusOK, code)
			assert.False(t, aborted)
		}
	})

	t.Run("admin_not_throttled", func(t *testing.T) {
		user := &model.User{Id: 90003, Role: model.RoleAdminUser, Quota: 0}
		for i := 0; i < 5; i++ {
			code, _, aborted := run(user)
			assert.Equal(t, http.StatusOK, code)
			assert.False(t, aborted)
		}
	})

	t.Run("noop_when_not_stricter_than_global", func(t *testing.T) {
		config.LowBalanceRelayRateLimitNum = config.GlobalRelayRateLimitNum
		defer func() { config.LowBalanceRelayRateLimitNum = 2 }()

		user := &model.User{Id: 90004, Role: model.RoleCommonUser, Quota: 0}
		for i := 0; i < 10; i++ {
			code, _, aborted := run(user)
			assert.Equal(t, http.StatusOK, code)
			assert.False(t, aborted)
		}
	})

	t.Run("rate_limit_disabled_flag_disables_limiter", func(t *testing.T) {
		// DEBUG must NOT affect rate limiting; only the explicit RATE_LIMIT_DISABLED
		// toggle bypasses the limiter.
		config.RateLimitDisabled = true
		defer func() { config.RateLimitDisabled = false }()

		user := &model.User{Id: 90005, Role: model.RoleCommonUser, Quota: 0}
		for i := 0; i < 10; i++ {
			code, _, aborted := run(user)
			assert.Equal(t, http.StatusOK, code)
			assert.False(t, aborted)
		}
	})

	t.Run("engages_when_only_duration_is_stricter", func(t *testing.T) {
		// Same request count as the global limit, but a much longer window, so the
		// effective rate is stricter. Count-only gating would treat this as a no-op;
		// the effective-rate gate must still engage the limiter.
		origG, origGD := config.GlobalRelayRateLimitNum, config.GlobalRelayRateLimitDuration
		origN, origD := config.LowBalanceRelayRateLimitNum, config.LowBalanceRelayRateLimitDuration
		defer func() {
			config.GlobalRelayRateLimitNum, config.GlobalRelayRateLimitDuration = origG, origGD
			config.LowBalanceRelayRateLimitNum, config.LowBalanceRelayRateLimitDuration = origN, origD
		}()
		config.GlobalRelayRateLimitNum = 3
		config.GlobalRelayRateLimitDuration = 10
		config.LowBalanceRelayRateLimitNum = 3           // identical count to the global limit
		config.LowBalanceRelayRateLimitDuration = 100000 // longer window => stricter effective rate

		user := &model.User{Id: 90006, Role: model.RoleCommonUser, Quota: 0}
		for i := 0; i < 3; i++ {
			code, _, aborted := run(user)
			assert.Equal(t, http.StatusOK, code)
			assert.False(t, aborted)
		}
		code, body, aborted := run(user)
		assert.Equal(t, http.StatusTooManyRequests, code)
		assert.True(t, aborted)
		assert.Contains(t, body, "balance")
	})

	t.Run("missing_user_object_is_noop", func(t *testing.T) {
		for i := 0; i < 10; i++ {
			code, _, aborted := run(nil)
			assert.Equal(t, http.StatusOK, code)
			assert.False(t, aborted)
		}
	})
}
