package controller

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"syscall"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor/openai_compatible"
)

// incidentHealthReadBody yields the supplied synthetic error without provider content.
type incidentHealthReadBody struct {
	err error
}

// Read returns zero bytes and the fixture error for the actual buffered response handler.
func (b *incidentHealthReadBody) Read([]byte) (int, error) {
	return 0, b.err
}

// Close releases the synthetic body without side effects.
func (b *incidentHealthReadBody) Close() error {
	return nil
}

// incidentHealthReadError calls the actual buffered handler with synthetic body failure.
func incidentHealthReadError(t *testing.T, rawErr error) (*http.Response, bool) {
	t.Helper()
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       &incidentHealthReadBody{err: rawErr},
	}
	relayErr, usage := openai_compatible.Handler(c, resp, 0, "synthetic-incident-model")
	require.NotNil(t, relayErr)
	require.Nil(t, usage)
	require.Equal(t, "read_response_body_failed", relayErr.Code)
	require.Equal(t, http.StatusInternalServerError, relayErr.StatusCode)
	require.Empty(t, recorder.Body.String(), "upstream failures must not send partial completion output")
	require.False(t, c.Writer.Written(), "the handler must leave downstream output uncommitted")
	require.ErrorIs(t, relayErr.RawError, rawErr)
	return resp, countsAgainstChannelHealth(relayErr)
}

// TestIncidentBodyResetHealthRegression requires a provider TCP reset to count as channel evidence.
func TestIncidentBodyResetHealthRegression(t *testing.T) {
	rawErr := &net.OpError{Op: "read", Net: "tcp", Err: syscall.ECONNRESET}
	_, counted := incidentHealthReadError(t, rawErr)
	require.True(t, counted, "an upstream body-read TCP reset is channel evidence, not an adaptor conversion fault")
}

// TestIncidentBodyResetHealthControls excludes caller cancellation and local conversion errors.
func TestIncidentBodyResetHealthControls(t *testing.T) {
	t.Run("caller_cancellation_is_not_channel_evidence", func(t *testing.T) {
		_, counted := incidentHealthReadError(t, context.Canceled)
		require.False(t, counted)
	})
	t.Run("caller_deadline_is_not_channel_evidence", func(t *testing.T) {
		_, counted := incidentHealthReadError(t, context.DeadlineExceeded)
		require.False(t, counted)
	})
	t.Run("local_conversion_fault_is_not_channel_evidence", func(t *testing.T) {
		conversionErr := &strconv.NumError{Func: "Atoi", Num: "synthetic", Err: strconv.ErrSyntax}
		normalized := openai_compatible.ErrorWrapper(conversionErr, "convert_request_failed", http.StatusInternalServerError)
		require.False(t, countsAgainstChannelHealth(normalized))
	})
	t.Run("genuine_provider_5xx_is_channel_evidence", func(t *testing.T) {
		normalized := openai_compatible.ErrorWrapper(io.ErrUnexpectedEOF, "read_response_body_failed", http.StatusInternalServerError)
		normalized.Type = "upstream_error"
		require.True(t, countsAgainstChannelHealth(normalized))
	})
	t.Run("no_failure_is_not_channel_evidence", func(t *testing.T) {
		require.False(t, countsAgainstChannelHealth(nil))
	})
}
