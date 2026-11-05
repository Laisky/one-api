package controller

import (
	"context"
	stderrors "errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Laisky/errors/v2"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/errkind"
	"github.com/Laisky/one-api/relay/model"
)

// trackedDeliveryContext builds a request whose downstream writer fails every
// body write after committing headers, observed by an installed delivery tracker.
func trackedDeliveryContext(t *testing.T, ctx context.Context, contentType string) (*gin.Context, *deliveryTracker, func(), error) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil).WithContext(ctx)
	cause := stderrors.New("synthetic broken client connection")
	c.Writer = failedClientDelivery{c.Writer, cause}
	tracker, release := trackDelivery(c)
	if contentType != "" {
		c.Writer.Header().Set("Content-Type", contentType)
	}
	_, err := c.Writer.Write([]byte("partial"))
	require.ErrorIs(t, err, cause)
	return c, tracker, release, cause
}

// TestAttributeDeliveryFailure verifies the relay-boundary delivery contract:
// a concealed write failure becomes a reported delivery failure, a provider-
// shaped error caused by the write is re-attributed to the gateway without a
// channel-health penalty, and unrelated or client-cancel failures are kept.
func TestAttributeDeliveryFailure(t *testing.T) {
	t.Run("concealed stream failure is reported", func(t *testing.T) {
		c, tracker, release, cause := trackedDeliveryContext(t, context.Background(), "text/event-stream")
		got := attributeDeliveryFailure(c, tracker, nil)
		release()
		require.NotNil(t, got)
		require.True(t, model.IsDeliveryFailure(got))
		require.Equal(t, model.DeliveryCodeStream, got.Code)
		require.ErrorIs(t, got.RawError, cause)
		require.Equal(t, errkind.Server, relayFailureKind(got))
		require.False(t, countsAgainstChannelHealth(got))
		_, stillTracked := c.Writer.(*deliveryTracker)
		require.False(t, stillTracked, "the tracker is removed after the attempt")
	})
	t.Run("concealed failure keeps client cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		c, tracker, release, cause := trackedDeliveryContext(t, ctx, "application/json")
		defer release()
		got := attributeDeliveryFailure(c, tracker, nil)
		require.Equal(t, model.DeliveryCodeResponse, got.Code)
		require.ErrorIs(t, got.RawError, cause)
		require.ErrorIs(t, got.RawError, context.Canceled)
		require.True(t, isClientContextCancel(got.StatusCode, got.RawError), "a client abort stays a client abort")
		require.False(t, countsAgainstChannelHealth(got))
	})
	t.Run("provider-shaped write failure is re-attributed", func(t *testing.T) {
		c, tracker, release, cause := trackedDeliveryContext(t, context.Background(), "text/event-stream")
		defer release()
		// Shaped like the Claude and AWS Claude handlers' write failures.
		provider := &model.ErrorWithStatusCode{StatusCode: http.StatusBadGateway, Error: model.Error{
			Type: "upstream_error", Code: "claude_response_failed", Message: "Claude response failed",
			RawError: errors.Wrap(errors.Wrap(cause, "write Claude response"), "Claude HTTP response")}}
		require.True(t, countsAgainstChannelHealth(provider), "fixture reproduces the provider-shaped penalty")
		got := attributeDeliveryFailure(c, tracker, provider)
		require.True(t, model.IsDeliveryFailure(got))
		require.ErrorIs(t, got.RawError, cause)
		require.Contains(t, got.RawError.Error(), "Claude HTTP response", "adaptor context is retained")
		require.Equal(t, errkind.Server, relayFailureKind(got))
		require.False(t, countsAgainstChannelHealth(got))
	})
	t.Run("independent provider failure is kept", func(t *testing.T) {
		c, tracker, release, _ := trackedDeliveryContext(t, context.Background(), "text/event-stream")
		defer release()
		provider := &model.ErrorWithStatusCode{StatusCode: http.StatusBadGateway, Error: model.Error{
			Type: "upstream_error", Code: "provider_failed", RawError: errors.New("provider reset the stream")}}
		require.Same(t, provider, attributeDeliveryFailure(c, tracker, provider))
	})
	t.Run("client cancellation failure is kept", func(t *testing.T) {
		c, tracker, release, cause := trackedDeliveryContext(t, context.Background(), "text/event-stream")
		defer release()
		canceled := &model.ErrorWithStatusCode{StatusCode: http.StatusInternalServerError, Error: model.Error{
			Type: model.ErrorTypeOneAPI, Code: "read_stream_failed", RawError: stderrors.Join(cause, context.Canceled)}}
		require.Same(t, canceled, attributeDeliveryFailure(c, tracker, canceled))
	})
	t.Run("existing delivery failure is kept", func(t *testing.T) {
		c, tracker, release, cause := trackedDeliveryContext(t, context.Background(), "")
		defer release()
		delivery := model.NewDeliveryError(cause, model.DeliveryCodeResponseBody)
		require.Same(t, delivery, attributeDeliveryFailure(c, tracker, delivery))
	})
	t.Run("successful delivery stays successful", func(t *testing.T) {
		gin.SetMode(gin.TestMode)
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
		tracker, release := trackDelivery(c)
		_, err := c.Writer.WriteString("delivered")
		require.NoError(t, err)
		require.NoError(t, http.NewResponseController(c.Writer).Flush(), "writer controls still reach the connection")
		require.Nil(t, attributeDeliveryFailure(c, tracker, nil))
		release()
		require.Equal(t, "delivered", recorder.Body.String())
	})
}

// TestDeliveryFailureSpoofResistance verifies that a provider error echoing a
// delivery code is never treated as a local delivery failure.
func TestDeliveryFailureSpoofResistance(t *testing.T) {
	spoofed := &model.ErrorWithStatusCode{StatusCode: http.StatusBadGateway, Error: model.Error{Type: "provider_error", Code: model.DeliveryCodeStream}}
	require.False(t, model.IsDeliveryFailure(spoofed))
	require.False(t, model.IsDeliveryFailure(&model.ErrorWithStatusCode{StatusCode: http.StatusBadRequest, Error: model.Error{Type: model.ErrorTypeOneAPI, Code: model.DeliveryCodeStream}}))
	require.False(t, model.IsDeliveryFailure(nil))
	require.True(t, model.IsDeliveryFailure(model.NewDeliveryError(nil, model.DeliveryCodeResponse)))
}
