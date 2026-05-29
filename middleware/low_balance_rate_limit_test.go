package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

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
		num       int
		dur       int64
		global    int
		threshold float64
		perUnit   float64
		debug     bool
	}{
		num:       config.LowBalanceRelayRateLimitNum,
		dur:       config.LowBalanceRelayRateLimitDuration,
		global:    config.GlobalRelayRateLimitNum,
		threshold: config.LowBalanceThreshold,
		perUnit:   config.QuotaPerUnit,
		debug:     config.DebugEnabled,
	}
	defer func() {
		config.LowBalanceRelayRateLimitNum = orig.num
		config.LowBalanceRelayRateLimitDuration = orig.dur
		config.GlobalRelayRateLimitNum = orig.global
		config.LowBalanceThreshold = orig.threshold
		config.QuotaPerUnit = orig.perUnit
		config.DebugEnabled = orig.debug
	}()

	// Force the in-memory limiter path: redisEnabled defaults to true but RDB is
	// nil in unit tests (InitRedisClient never runs here).
	origRedis := common.IsRedisEnabled()
	common.SetRedisEnabled(false)
	defer common.SetRedisEnabled(origRedis)

	// Stricter-than-global config so the limiter engages.
	config.DebugEnabled = false
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

	t.Run("debug_mode_disables_limiter", func(t *testing.T) {
		config.DebugEnabled = true
		defer func() { config.DebugEnabled = false }()

		user := &model.User{Id: 90005, Role: model.RoleCommonUser, Quota: 0}
		for i := 0; i < 10; i++ {
			code, _, aborted := run(user)
			assert.Equal(t, http.StatusOK, code)
			assert.False(t, aborted)
		}
	})

	t.Run("missing_user_object_is_noop", func(t *testing.T) {
		for i := 0; i < 10; i++ {
			code, _, aborted := run(nil)
			assert.Equal(t, http.StatusOK, code)
			assert.False(t, aborted)
		}
	})
}
