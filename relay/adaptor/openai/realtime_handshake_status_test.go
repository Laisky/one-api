package openai

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/Laisky/errors/v2"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/Laisky/zap"
	"github.com/Laisky/zap/zapcore"
	"github.com/Laisky/zap/zaptest/observer"
	"github.com/stretchr/testify/require"
)

// handshakeStatusPrivateBody is a provider handshake body marker that must never
// be read into a log entry.
const handshakeStatusPrivateBody = "private-handshake-body"

// handshakeCloseFailingBody is a handshake response body whose Close fails, so
// the close-error diagnostic can be observed.
type handshakeCloseFailingBody struct {
	io.Reader
	closed bool
}

// Close records the call and returns a deterministic close failure.
//
// Parameters: none.
//
// Returns:
//   - error: always a close failure.
func (b *handshakeCloseFailingBody) Close() error {
	b.closed = true
	return errors.New("handshake body close failed")
}

// newHandshakeStatusLogger returns a DEBUG logger whose entries are observed.
//
// Parameters:
//   - t: the running test.
//
// Returns:
//   - glog.Logger: the logger to pass to the helper.
//   - *observer.ObservedLogs: the captured entries.
func newHandshakeStatusLogger(t *testing.T) (glog.Logger, *observer.ObservedLogs) {
	t.Helper()
	core, observed := observer.New(zapcore.DebugLevel)
	lg, err := glog.NewConsoleWithName("handshake-status-test", glog.LevelDebug,
		zap.WrapCore(func(zapcore.Core) zapcore.Core { return core }))
	require.NoError(t, err)
	return lg, observed
}

// TestWebSocketHandshakeFailureStatusLogsCloseError verifies that a failing
// handshake body close is logged once at DEBUG with the error and the status,
// without the provider body, and that the provider status is still reported.
//
// Parameters:
//   - t: the running test.
//
// Returns: none.
func TestWebSocketHandshakeFailureStatusLogsCloseError(t *testing.T) {
	lg, observed := newHandshakeStatusLogger(t)
	body := &handshakeCloseFailingBody{Reader: strings.NewReader(handshakeStatusPrivateBody)}

	status, fromUpstream := WebSocketHandshakeFailureStatus(lg, &http.Response{StatusCode: http.StatusTooManyRequests, Body: body})
	require.Equal(t, http.StatusTooManyRequests, status)
	require.True(t, fromUpstream)
	require.True(t, body.closed)

	entries := observed.FilterMessage("close upstream websocket handshake response body failed").All()
	require.Len(t, entries, 1)
	require.Equal(t, zapcore.DebugLevel, entries[0].Level)
	fields := entries[0].ContextMap()
	require.EqualValues(t, http.StatusTooManyRequests, fields["status"])
	require.Equal(t, "handshake body close failed", fields["error"])
	require.Equal(t, 1, observed.Len(), "only the close diagnostic is logged")
	require.NotContains(t, fmt.Sprint(observed.All()), handshakeStatusPrivateBody)
}

// TestWebSocketHandshakeFailureStatusClassification verifies the status and
// provenance reported for each handshake outcome: only a provider answer in
// 400..599 is provider-attributed; no response, a missing body, a non-error
// answer, or an out-of-range status yields one-api's own 502. A successful
// close logs nothing.
//
// Parameters:
//   - t: the running test.
//
// Returns: none.
func TestWebSocketHandshakeFailureStatusClassification(t *testing.T) {
	for _, tc := range []struct {
		name         string
		response     *http.Response
		wantStatus   int
		wantUpstream bool
	}{
		{"no_response", nil, http.StatusBadGateway, false},
		{"no_body_503", &http.Response{StatusCode: http.StatusServiceUnavailable}, http.StatusServiceUnavailable, true},
		{"switching_protocols", &http.Response{StatusCode: http.StatusSwitchingProtocols, Body: http.NoBody}, http.StatusBadGateway, false},
		{"ok", &http.Response{StatusCode: http.StatusOK, Body: http.NoBody}, http.StatusBadGateway, false},
		{"redirect", &http.Response{StatusCode: http.StatusFound, Body: http.NoBody}, http.StatusBadGateway, false},
		{"lower_bound_400", &http.Response{StatusCode: http.StatusBadRequest, Body: http.NoBody}, http.StatusBadRequest, true},
		{"unauthorized", &http.Response{StatusCode: http.StatusUnauthorized, Body: http.NoBody}, http.StatusUnauthorized, true},
		{"upper_bound_599", &http.Response{StatusCode: 599, Body: http.NoBody}, 599, true},
		{"out_of_range_600", &http.Response{StatusCode: 600, Body: http.NoBody}, http.StatusBadGateway, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			lg, observed := newHandshakeStatusLogger(t)
			status, fromUpstream := WebSocketHandshakeFailureStatus(lg, tc.response)
			require.Equal(t, tc.wantStatus, status)
			require.Equal(t, tc.wantUpstream, fromUpstream)
			require.Zero(t, observed.Len(), "a clean close logs nothing")
		})
	}
}
