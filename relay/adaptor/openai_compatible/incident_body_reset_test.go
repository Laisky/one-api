package openai_compatible_test

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httptrace"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor/deepseek"
	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/relaymode"
)

const incidentCompleteBody = `{"choices":[{"index":0,"message":{"role":"assistant","content":"synthetic local reply"},"finish_reason":"stop"}],"usage":{"prompt_tokens":2,"completion_tokens":3,"total_tokens":5}}`
const incidentBodyPrefix = `{"choices":[{"index":0,"message":{"role":"assistant","content":"synthetic`

// incidentSocketFixture owns a localhost HTTP/1 server and a deterministic abort gate.
type incidentSocketFixture struct {
	server  *httptest.Server
	client  *http.Client
	release chan struct{}
	once    sync.Once
	errors  chan error
}

// unblock permits the fixture to close its target connection after a body read begins.
func (f *incidentSocketFixture) unblock() { f.once.Do(func() { close(f.release) }) }

// newIncidentSocketFixture creates an upstream that succeeds, resets, or truncates a body.
func newIncidentSocketFixture(t *testing.T, mode string, prefix bool) *incidentSocketFixture {
	t.Helper()
	f := &incidentSocketFixture{release: make(chan struct{}), errors: make(chan error, 1)}
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/warm" || mode == "success" {
			w.Header().Set("Content-Type", "application/json")
			_, err := io.WriteString(w, incidentCompleteBody)
			if err != nil {
				f.errors <- err
			}
			return
		}
		conn, buf, err := w.(http.Hijacker).Hijack()
		if err != nil {
			f.errors <- err
			return
		}
		defer conn.Close()
		// The declared body is longer than either intentionally incomplete body.
		_, err = io.WriteString(buf, "HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Length: 1000\r\n\r\n")
		if err == nil && prefix {
			_, err = io.WriteString(buf, incidentBodyPrefix)
		}
		if err == nil {
			err = buf.Flush()
		}
		if err != nil {
			f.errors <- err
			return
		}
		select {
		case <-f.release:
		case <-r.Context().Done():
			return
		}
		if mode == "rst" {
			if err := conn.(*net.TCPConn).SetLinger(0); err != nil {
				f.errors <- err
			}
		}
	}))
	transport := &http.Transport{MaxConnsPerHost: 1, MaxIdleConnsPerHost: 1}
	f.client = &http.Client{Transport: transport, Timeout: 5 * time.Second}
	t.Cleanup(func() {
		f.unblock()
		transport.CloseIdleConnections()
		f.server.Close()
		select {
		case err := <-f.errors:
			t.Errorf("local upstream fixture failed: %v", err)
		default:
		}
	})
	return f
}

// incidentObservedBody records actual body receipt and explicit Close calls.
type incidentObservedBody struct {
	io.ReadCloser
	fixture *incidentSocketFixture
	prefix  bool
	bytes   int
	closes  atomic.Int32
}

// Read releases a zero-body abort before reading or a prefix abort after receiving bytes.
func (b *incidentObservedBody) Read(p []byte) (int, error) {
	if !b.prefix {
		b.fixture.unblock()
	}
	n, err := b.ReadCloser.Read(p)
	b.bytes += n
	if n > 0 {
		b.fixture.unblock()
	}
	return n, err
}

// Close counts explicit ownership cleanup and closes the underlying response body.
func (b *incidentObservedBody) Close() error {
	b.closes.Add(1)
	return b.ReadCloser.Close()
}

// incidentResponse returns a target response and proves whether its HTTP/1 socket was reused.
func incidentResponse(t *testing.T, f *incidentSocketFixture, reused, prefix bool) *http.Response {
	t.Helper()
	var warmConn net.Conn
	if reused {
		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, f.server.URL+"/warm", nil)
		require.NoError(t, err)
		req = req.WithContext(httptrace.WithClientTrace(req.Context(), &httptrace.ClientTrace{GotConn: func(info httptrace.GotConnInfo) { warmConn = info.Conn }}))
		resp, err := f.client.Do(req)
		require.NoError(t, err)
		_, err = io.Copy(io.Discard, resp.Body)
		require.NoError(t, err)
		require.NoError(t, resp.Body.Close())
	}
	var target httptrace.GotConnInfo
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, f.server.URL+"/target", strings.NewReader("synthetic local input"))
	require.NoError(t, err)
	req = req.WithContext(httptrace.WithClientTrace(req.Context(), &httptrace.ClientTrace{GotConn: func(info httptrace.GotConnInfo) { target = info }}))
	resp, err := f.client.Do(req)
	require.NoError(t, err, "the fixture must deliver HTTP headers before aborting")
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, 1, resp.ProtoMajor)
	require.Equal(t, reused, target.Reused)
	if reused {
		require.Same(t, warmConn, target.Conn, "the warmed socket must actually be reused")
	}
	body := &incidentObservedBody{ReadCloser: resp.Body, fixture: f, prefix: prefix}
	resp.Body = body
	// Cleanup happens after assertions and cannot conceal missing handler ownership.
	t.Cleanup(func() { require.NoError(t, body.ReadCloser.Close()) })
	return resp
}

// TestIncidentBodyResetNonstreamControls distinguishes a response reset from success and truncated FIN.
func TestIncidentBodyResetNonstreamControls(t *testing.T) {
	for _, reused := range []bool{false, true} {
		name := "fresh"
		if reused {
			name = "reused"
		}
		for _, tc := range []struct {
			name, mode string
			prefix     bool
		}{
			{"success", "success", false},
			{"rst_zero", "rst", false},
			{"rst_prefix", "rst", true},
			{"fin_zero", "fin", false},
			{"fin_prefix", "fin", true},
		} {
			t.Run(name+"/"+tc.name, func(t *testing.T) {
				f := newIncidentSocketFixture(t, tc.mode, tc.prefix)
				resp := incidentResponse(t, f, reused, tc.prefix)
				body := resp.Body.(*incidentObservedBody)
				out := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(out)
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
				provider := &deepseek.Adaptor{}
				usage, apiErr := provider.DoResponse(c, resp, &meta.Meta{ChannelType: channeltype.DeepSeek, ActualModelName: "deepseek-flash", Mode: relaymode.ChatCompletions})
				if tc.mode == "success" {
					require.Nil(t, apiErr)
					require.NotNil(t, usage)
					require.Equal(t, 5, usage.TotalTokens)
					require.Contains(t, out.Body.String(), "synthetic local reply")
					require.EqualValues(t, 1, body.closes.Load())
					return
				}
				require.NotNil(t, apiErr)
				require.Equal(t, "read_response_body_failed", apiErr.Code)
				require.Equal(t, http.StatusInternalServerError, apiErr.StatusCode)
				require.Nil(t, usage)
				require.False(t, c.Writer.Written(), "partial upstream content must not reach the caller")
				require.Empty(t, out.Body.String())
				if tc.prefix {
					require.Equal(t, len(incidentBodyPrefix), body.bytes)
				} else {
					require.Zero(t, body.bytes)
				}
				if tc.mode == "rst" {
					require.True(t, errors.Is(apiErr.RawError, syscall.ECONNRESET), "must reproduce a real socket reset, not merely an EOF: %v", apiErr.RawError)
				} else {
					require.ErrorIs(t, apiErr.RawError, io.ErrUnexpectedEOF)
					require.False(t, errors.Is(apiErr.RawError, syscall.ECONNRESET))
				}
			})
		}
	}
}

// TestIncidentBodyResetReadErrorCloses requires cleanup after an upstream body-read failure.
func TestIncidentBodyResetReadErrorCloses(t *testing.T) {
	f := newIncidentSocketFixture(t, "rst", true)
	resp := incidentResponse(t, f, true, true)
	body := resp.Body.(*incidentObservedBody)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	usage, apiErr := (&deepseek.Adaptor{}).DoResponse(c, resp, &meta.Meta{ChannelType: channeltype.DeepSeek, ActualModelName: "deepseek-flash", Mode: relaymode.ChatCompletions})
	require.NotNil(t, apiErr)
	require.Nil(t, usage)
	require.True(t, errors.Is(apiErr.RawError, syscall.ECONNRESET))
	require.EqualValues(t, 1, body.closes.Load(), "the response handler must explicitly close the failed upstream body")
}
