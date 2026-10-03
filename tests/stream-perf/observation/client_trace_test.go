package observation

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"runtime/trace"
	"strings"
	"testing"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/stretchr/testify/require"
)

// TestClientTraceExplicitListenerBounds keeps disabled tracing inert and rejects nonlocal or ambiguous bindings.
func TestClientTraceExplicitListenerBounds(t *testing.T) {
	disabled, err := StartClientTrace("")
	require.NoError(t, err)
	require.Nil(t, disabled)
	require.NoError(t, disabled.Close())
	for _, address := range []string{"0.0.0.0:0", "localhost:0", "[::1]:0", "127.0.0.1:01", "127.0.0.1:65536", "127.0.0.1:-1", "garbage"} {
		_, err := StartClientTrace(address)
		require.Error(t, err, address)
	}
	server, err := StartClientTrace("127.0.0.1:0")
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(server.Address(), "127.0.0.1:"))
	_, err = StartClientTrace(server.Address())
	require.Error(t, err)
	require.NoError(t, server.Close())
	require.NoError(t, server.Close())
	listener, err := net.Listen("tcp4", server.Address())
	require.NoError(t, err)
	require.NoError(t, listener.Close())
}

// TestClientTraceRejectsOtherRoutesAndDurations protects all pprof handlers except a bounded execution trace.
func TestClientTraceRejectsOtherRoutesAndDurations(t *testing.T) {
	for _, url := range []string{"/debug/pprof/heap", "/debug/pprof/trace", "/debug/pprof/trace?seconds=0", "/debug/pprof/trace?seconds=11", "/debug/pprof/trace?seconds=1.5", "/debug/pprof/trace?seconds=1&seconds=2", "/debug/pprof/trace?seconds=1&other=1", "/debug/pprof/trace?seconds=%zz"} {
		request := httptest.NewRequest("GET", url, nil)
		response := httptest.NewRecorder()
		clientTraceHandler(response, request)
		require.GreaterOrEqual(t, response.Code, 400, url)
	}
	response := httptest.NewRecorder()
	clientTraceHandler(response, httptest.NewRequest("POST", "/debug/pprof/trace?seconds=1", nil))
	require.Equal(t, http.StatusNotFound, response.Code)
	for _, seconds := range []string{"1", "5", "10"} {
		_, err := netQuerySeconds("seconds=" + seconds)
		require.NoError(t, err)
	}
}

// TestClientObservationIdentityAndClock retains the identical sample timestamp and emits no request data.
func TestClientObservationIdentityAndClock(t *testing.T) {
	var category, message string
	emit := func(_ context.Context, c, m string) { category, message = c, m }
	times, err := observeClient([]int64{10}, 32, func() (int64, error) { return 20, nil }, func() bool { return true }, emit)
	require.NoError(t, err)
	require.Equal(t, []int64{10, 20}, times)
	require.Equal(t, "oneapi.sse.client", category)
	require.Equal(t, "32/1/observe/20", message)
	category, message = "", ""
	_, err = observeClient(nil, 32, func() (int64, error) { return 30, nil }, func() bool { return false }, emit)
	require.NoError(t, err)
	require.Empty(t, message)
	for _, id := range []int{-1, 1, 8192} {
		_, err = observeClient(nil, id, func() (int64, error) { t.Fatal("invalid identity read clock"); return 0, nil }, func() bool { return true }, emit)
		require.Error(t, err)
	}
	_, err = observeClient([]int64{50}, 32, func() (int64, error) { return 20, nil }, func() bool { return true }, emit)
	require.Error(t, err)
	_, err = observeClient(make([]int64, MaxFrames), 32, func() (int64, error) { t.Fatal("full buffer read clock"); return 0, nil }, func() bool { return true }, emit)
	require.Error(t, err)
	injected := errors.New("clock failure")
	_, err = observeClient(nil, 32, func() (int64, error) { return 0, injected }, func() bool { return true }, emit)
	require.ErrorIs(t, err, injected)
	require.Empty(t, message, "invalid observations must not produce successful markers")
}

// TestClientTraceActuallyCaptures checks the real bounded HTTP endpoint rather than only a mocked handler.
func TestClientTraceActuallyCaptures(t *testing.T) {
	server, err := StartClientTrace("127.0.0.1:0")
	require.NoError(t, err)
	defer func() { require.NoError(t, server.Close()) }()
	client := &http.Client{Timeout: 3 * time.Second, Transport: &http.Transport{Proxy: nil}}
	defer client.CloseIdleConnections()
	type outcome struct {
		data   []byte
		err    error
		status int
	}
	finished := make(chan outcome, 1)
	go func() {
		response, err := client.Get("http://" + server.Address() + "/debug/pprof/trace?seconds=1")
		if err != nil {
			finished <- outcome{err: err}
			return
		}
		data, readErr := io.ReadAll(io.LimitReader(response.Body, 16<<20))
		closeErr := response.Body.Close()
		finished <- outcome{data: data, err: errors.Join(readErr, closeErr), status: response.StatusCode}
	}()
	deadline := time.Now().Add(2 * time.Second)
	for !trace.IsEnabled() && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	require.True(t, trace.IsEnabled(), "client endpoint did not start its trace")
	_, err = ObserveClient(nil, 32)
	require.NoError(t, err)
	result := <-finished
	require.NoError(t, result.err)
	require.Equal(t, http.StatusOK, result.status)
	require.Greater(t, len(result.data), 16)
	require.Less(t, len(result.data), 16<<20)
	require.True(t, strings.HasPrefix(string(result.data[:16]), "go 1."), "expected binary Go trace header")
	require.Contains(t, string(result.data), "oneapi.sse.client")
}
