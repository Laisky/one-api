package controller

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/Laisky/errors/v2"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/model"
)

// TestProviderRejectedVideoCreation pins the admission-refusal allowlist: only
// provider-attributed 400/401/402/403/404/413/422/429 answers prove that no paid
// job was created; 408, 409, every 5xx, local errors, cancellations and
// generated-output shapes stay ambiguous.
func TestProviderRejectedVideoCreation(t *testing.T) {
	definitive := map[int]bool{
		http.StatusBadRequest: true, http.StatusUnauthorized: true, http.StatusPaymentRequired: true,
		http.StatusForbidden: true, http.StatusNotFound: true, http.StatusRequestEntityTooLarge: true,
		http.StatusUnprocessableEntity: true, http.StatusTooManyRequests: true,
	}
	for status := 100; status < 600; status++ {
		t.Run(strconv.Itoa(status), func(t *testing.T) {
			upstream := providerStatus(status, "provider_code")(1)
			require.Equal(t, definitive[status], providerRejectedVideoCreation(upstream),
				"provider-attributed status %d", status)

			local := *upstream
			local.FromUpstream = false
			require.False(t, providerRejectedVideoCreation(&local), "a local error proves nothing about the provider")
		})
	}

	require.False(t, providerRejectedVideoCreation(nil))
	require.True(t, providerRejectedVideoCreation(issue625QuotaRejection(1)), "issue #625 envelope is a refusal")

	cancelled := issue625QuotaRejection(1)
	cancelled.RawError = errors.Wrap(context.Canceled, "client went away")
	require.False(t, providerRejectedVideoCreation(cancelled), "a cancelled attempt stays ambiguous")
	expired := issue625QuotaRejection(1)
	expired.RawError = errors.Wrap(context.DeadlineExceeded, "attempt deadline")
	require.False(t, providerRejectedVideoCreation(expired), "a timed-out attempt stays ambiguous")

	generated := &model.ErrorWithStatusCode{StatusCode: http.StatusBadRequest, Error: model.Error{
		Code: "output_parse_failed", Message: "generated output that could not be parsed", FromUpstream: true}}
	require.False(t, providerRejectedVideoCreation(generated), "generated output implies provider work")

	wrapped := openai.UpstreamErrorWrapper(errors.New("provider quota exhausted"), "AccountQuotaExceeded", http.StatusTooManyRequests)
	require.True(t, providerRejectedVideoCreation(wrapped), "the adaptor's provider-status wrapper carries provenance")
}

// newForwardedVideoContext builds a POST context for path bound to channelID
// whose creation may already have been forwarded. Parameters: path is the
// request route and channelID the bound channel. It returns the gin context.
func newForwardedVideoContext(path string, channelID int) *gin.Context {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, path, nil)
	c.Set(ctxkey.ChannelId, channelID)
	c.Set(ctxkey.UpstreamRequestPossiblyForwarded, true)
	return c
}

// TestVideoReplayVerdictBoundToAttempt pins that a recorded rejection vouches
// only for the channel attempt that produced it: a later attempt on another
// channel, a later ambiguous answer, or an accepted job restores the veto.
func TestVideoReplayVerdictBoundToAttempt(t *testing.T) {
	for _, route := range videoFailoverRoutes {
		t.Run(route.path, func(t *testing.T) {
			c := newForwardedVideoContext(route.path, 7)
			require.True(t, videoReplayUnsafe(c), "no verdict yet: a forwarded creation is unsafe")

			require.NoError(t, shouldRetry(c, issue625QuotaRejection(7)))
			require.False(t, videoReplayUnsafe(c), "the provider refused this attempt before admission")

			c.Set(ctxkey.ChannelId, 8)
			require.True(t, videoReplayUnsafe(c), "a verdict must not vouch for another channel's attempt")

			require.Error(t, shouldRetry(c, providerStatus(http.StatusServiceUnavailable, "overloaded")(8)))
			c.Set(ctxkey.ChannelId, 7)
			require.True(t, videoReplayUnsafe(c), "an ambiguous answer replaces the earlier rejection")

			require.NoError(t, shouldRetry(c, issue625QuotaRejection(7)))
			c.Set(adaptor.AsyncVideoAcceptedKey, true)
			require.True(t, videoReplayUnsafe(c), "an accepted job is never replayed")
			require.Error(t, shouldRetry(c, issue625QuotaRejection(7)))
		})
	}
}

// TestVideoReplayVerdictIgnoresOtherRoutes pins that non-creation routes keep
// their existing retry semantics and never receive a video verdict.
func TestVideoReplayVerdictIgnoresOtherRoutes(t *testing.T) {
	for _, tc := range []struct{ method, path string }{
		{http.MethodPost, "/v1/chat/completions"},
		{http.MethodGet, "/v1/videos/video_123"},
		{http.MethodDelete, "/v1/videos/video_123"},
	} {
		t.Run(tc.method+tc.path, func(t *testing.T) {
			c := newForwardedVideoContext(tc.path, 7)
			c.Request.Method = tc.method
			require.NoError(t, shouldRetry(c, issue625QuotaRejection(7)))
			_, recorded := c.Get(videoCreationRejectedChannelKey)
			require.False(t, recorded, "only paid video creations record a verdict")
			require.False(t, videoReplayUnsafe(c))
		})
	}
}
