package controller

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// TestAWSIdleNotificationBoundary ensures the notification targets an idle SDK
// callback after messageStart, not the initial event-stream header flush.
func TestAWSIdleNotificationBoundary(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	signal := make(chan struct{})
	writer := &awsSecurityWriter{ResponseWriter: c.Writer, mode: "cancel_while_idle", idleNotify: signal}
	writer.Flush()
	select {
	case <-signal:
		t.Fatal("initial header flush must not signal a decoded SDK event")
	default:
	}
	writer.Flush()
	select {
	case <-signal:
	default:
		t.Fatal("messageStart callback flush must release the idle notification")
	}
	require.NotPanics(t, func() { writer.Flush() }, "later flushes must not close the notification twice")
}
