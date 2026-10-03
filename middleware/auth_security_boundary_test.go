package middleware

import (
	"fmt"
	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestSecurityDeletedOwnerTokenDenied verifies deleted owners cannot reach relay handlers.
func TestSecurityDeletedOwnerTokenDenied(t *testing.T) {
	db := setupTokenAuthChannelSuffixTestDB(t)
	old := model.DB
	model.DB = db
	redis := common.IsRedisEnabled()
	common.SetRedisEnabled(false)
	t.Cleanup(func() { model.DB = old; common.SetRedisEnabled(redis) })
	require.NoError(t, db.Model(&model.User{}).Where("id = ?", 1).Update("status", model.UserStatusDeleted).Error)
	r := gin.New()
	r.Use(TokenAuth())
	r.GET("/v1/models", func(c *gin.Context) { c.Status(http.StatusNoContent) })
	req := httptest.NewRequest("GET", "/v1/models", nil)
	req.Header.Set("Authorization", "Bearer sk-admintoken")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusForbidden, w.Code)
}

// TestSecurityVideoCollectionDenied verifies provider-wide job collections never reach upstream distribution.
func TestSecurityVideoCollectionDenied(t *testing.T) {
	r := gin.New()
	r.Use(BindAsyncTaskChannel())
	r.GET("/v1/videos", func(c *gin.Context) { c.Status(http.StatusNoContent) })
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/v1/videos", nil))
	require.Equal(t, http.StatusForbidden, w.Code)
}

// TestSecurityLoginFailureBound verifies unique failed login identifiers have a bounded memory footprint.
func TestSecurityLoginFailureBound(t *testing.T) {
	loginFailTracker.Lock()
	old := loginFailTracker.entries
	oldOverflow := loginFailTracker.overflowUntil
	loginFailTracker.entries = make(map[string]*loginFailEntry)
	loginFailTracker.Unlock()
	t.Cleanup(func() {
		loginFailTracker.Lock()
		loginFailTracker.entries = old
		loginFailTracker.overflowUntil = oldOverflow
		loginFailTracker.Unlock()
	})
	for i := 0; i < 11000; i++ {
		RecordLoginFailure(fmt.Sprintf("missing-%d", i))
	}
	loginFailTracker.RLock()
	n := len(loginFailTracker.entries)
	loginFailTracker.RUnlock()
	require.LessOrEqual(t, n, 10000)
	require.True(t, HasLoginFailure("missing-10999"), "capacity exhaustion must fail closed")
}

// TestSecurityLoginFailureLifecycle verifies normal failure expiry and successful-login cleanup.
func TestSecurityLoginFailureLifecycle(t *testing.T) {
	const name = "lifecycle-fixture"
	ClearLoginFailure(name)
	RecordLoginFailure(name)
	require.True(t, HasLoginFailure(name))
	loginFailTracker.Lock()
	key := loginFailureKey(name)
	require.Len(t, key, 64)
	loginFailTracker.entries[key].lastFailAt = time.Now().Add(-2 * loginFailExpiry)
	loginFailTracker.Unlock()
	require.False(t, HasLoginFailure(name))
	pruneLoginFailEntries()
	loginFailTracker.RLock()
	_, exists := loginFailTracker.entries[key]
	loginFailTracker.RUnlock()
	require.False(t, exists)
	RecordLoginFailure(name)
	ClearLoginFailure(name)
	require.False(t, HasLoginFailure(name))
}
