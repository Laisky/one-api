package controller

import (
	"context"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

// TestShouldRetryContextCancellation ensures we do not retry when rawErr indicates
// a context cancellation even if status code is 500.
func TestShouldRetryContextCancellation(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(nil)

	err := shouldRetry(c, http.StatusInternalServerError, context.Canceled)
	assert.Error(t, err, "should not retry when context is canceled causing 5xx")
}

// TestShouldRetryDeadlineExceeded ensures we do not retry when rawErr indicates
// a deadline exceeded even if status code is 500.
func TestShouldRetryDeadlineExceeded(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(nil)

	err := shouldRetry(c, http.StatusInternalServerError, context.DeadlineExceeded)
	assert.Error(t, err, "should not retry when context deadline exceeded causing 5xx")
}

// TestShouldRetryNormal500 ensures we DO retry for a normal 500 without context cancellation.
func TestShouldRetryNormal500(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(nil)

	err := shouldRetry(c, http.StatusInternalServerError, nil)
	assert.NoError(t, err, "should retry for normal 500 error")
}

// TestShouldRetryClientError ensures client 400 errors still do not retry (except whitelisted codes).
func TestShouldRetryClientError(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(nil)

	err := shouldRetry(c, http.StatusBadRequest, nil)
	assert.Error(t, err, "should not retry for 400 bad request")
}
