package controller

import (
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	"github.com/songquanpeng/one-api/common/ctxkey"
	"github.com/songquanpeng/one-api/relay/model"
)

func TestShouldRetry_ClientAndAuthMatrix(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	cases := []struct {
		name        string
		status      int
		expectRetry bool
	}{
		{name: "400 bad request should not retry", status: http.StatusBadRequest, expectRetry: false},
		{name: "404 not found should not retry", status: http.StatusNotFound, expectRetry: false},
		{name: "413 capacity should retry", status: http.StatusRequestEntityTooLarge, expectRetry: true},
		{name: "429 rate limit should retry", status: http.StatusTooManyRequests, expectRetry: true},
		{name: "401 unauthorized should retry", status: http.StatusUnauthorized, expectRetry: true},
		{name: "403 forbidden should retry", status: http.StatusForbidden, expectRetry: true},
		{name: "500 server should retry", status: http.StatusInternalServerError, expectRetry: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c, _ := gin.CreateTestContext(nil)
			c.Set(ctxkey.SpecificChannelId, 0)
			err := shouldRetry(c, tc.status, nil)
			if tc.expectRetry {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
			}
		})
	}

	// When specific channel is pinned, never retry regardless of status
	c, _ := gin.CreateTestContext(nil)
	c.Set(ctxkey.SpecificChannelId, 42)
	assert.Error(t, shouldRetry(c, http.StatusTooManyRequests, nil))
}

func TestClassifyAuthLike(t *testing.T) {
	t.Parallel()
	t.Run("nil error", func(t *testing.T) {
		t.Parallel()
		assert.False(t, classifyAuthLike(nil))
	})

	t.Run("401/403 direct", func(t *testing.T) {
		t.Parallel()
		e1 := &model.ErrorWithStatusCode{StatusCode: http.StatusUnauthorized}
		e2 := &model.ErrorWithStatusCode{StatusCode: http.StatusForbidden}
		assert.True(t, classifyAuthLike(e1))
		assert.True(t, classifyAuthLike(e2))
	})

	t.Run("type-based", func(t *testing.T) {
		t.Parallel()
		for _, typ := range []model.ErrorType{
			model.ErrorTypeAuthentication,
			model.ErrorTypePermission,
			model.ErrorTypeInsufficientQuota,
			model.ErrorTypeForbidden,
		} {
			e := &model.ErrorWithStatusCode{Error: model.Error{Type: typ}}
			assert.True(t, classifyAuthLike(e), typ)
		}
	})

	t.Run("code-based", func(t *testing.T) {
		t.Parallel()
		for _, code := range []any{"invalid_api_key", "account_deactivated", "insufficient_quota"} {
			e := &model.ErrorWithStatusCode{Error: model.Error{Code: code}}
			assert.True(t, classifyAuthLike(e), code)
		}
	})

	t.Run("message-based", func(t *testing.T) {
		t.Parallel()
		msgs := []string{
			"API key not valid",
			"API KEY EXPIRED",
			"insufficient quota for this org",
			"已欠费，余额不足",
			"organization restricted",
		}
		for _, m := range msgs {
			e := &model.ErrorWithStatusCode{Error: model.Error{Message: m}}
			assert.True(t, classifyAuthLike(e), m)
		}
	})

	t.Run("non-auth server error", func(t *testing.T) {
		t.Parallel()
		e := &model.ErrorWithStatusCode{StatusCode: http.StatusInternalServerError, Error: model.Error{Message: "internal error"}}
		assert.False(t, classifyAuthLike(e))
	})
}
