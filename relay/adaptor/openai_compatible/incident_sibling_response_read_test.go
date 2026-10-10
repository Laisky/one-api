package openai_compatible_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"syscall"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/relay/adaptor/deepseek"
	compatible "github.com/Laisky/one-api/relay/adaptor/openai_compatible"
	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/relaymode"
)

// incidentSiblingBody supplies a deterministic body failure and counts ownership cleanup.
type incidentSiblingBody struct {
	reader   io.Reader
	readErr  error
	closeErr error
	closes   int
}

// Read returns synthetic bytes first, followed by the selected terminal read error.
func (b *incidentSiblingBody) Read(p []byte) (int, error) {
	n, err := b.reader.Read(p)
	if errors.Is(err, io.EOF) && b.readErr != nil {
		return n, b.readErr
	}
	return n, err
}

// Close records the call and returns a selected cleanup failure without external effects.
func (b *incidentSiblingBody) Close() error { b.closes++; return b.closeErr }

// incidentSiblingPath describes a production buffered response-reader entrypoint.
type incidentSiblingPath struct {
	name, readCode, successBody string
	status                      int
	run                         func(*gin.Context, *http.Response) *model.ErrorWithStatusCode
}

// incidentSiblingPaths includes DeepSeek chat, Claude conversion, JSON stream errors and shared siblings.
func incidentSiblingPaths() []incidentSiblingPath {
	deepseekPath := func(conversion, stream bool) func(*gin.Context, *http.Response) *model.ErrorWithStatusCode {
		return func(c *gin.Context, resp *http.Response) *model.ErrorWithStatusCode {
			c.Set(ctxkey.ClaudeMessagesConversion, conversion)
			_, apiErr := (&deepseek.Adaptor{}).DoResponse(c, resp, &meta.Meta{ChannelType: channeltype.DeepSeek, ActualModelName: "deepseek-flash", PromptTokens: 2, Mode: relaymode.ChatCompletions, IsStream: stream})
			return apiErr
		}
	}
	return []incidentSiblingPath{
		{"deepseek_chat", "read_response_body_failed", incidentCompleteBody, 200, deepseekPath(false, false)},
		{"deepseek_claude", "read_response_body_failed", incidentCompleteBody, 200, deepseekPath(true, false)},
		{"deepseek_stream_json", "read_error_response_failed", `{"error":{"type":"server_error","code":"synthetic_upstream_error","message":"synthetic local rejection"}}`, 502, deepseekPath(false, true)},
		{"embedding", "read_response_body_failed", `{"object":"list","data":[{"object":"embedding","index":0,"embedding":[0.1,0.2]}],"usage":{"prompt_tokens":2,"total_tokens":2}}`, 200, func(c *gin.Context, resp *http.Response) *model.ErrorWithStatusCode {
			apiErr, _ := compatible.EmbeddingHandler(c, resp)
			return apiErr
		}},
		{"thinking", "read_response_body_failed", incidentCompleteBody, 200, func(c *gin.Context, resp *http.Response) *model.ErrorWithStatusCode {
			apiErr, _ := compatible.HandlerWithThinking(c, resp, 2, "deepseek-flash")
			return apiErr
		}},
	}
}

// incidentSiblingContext provides a synthetic request and uncommitted downstream writer.
func incidentSiblingContext() (*gin.Context, *httptest.ResponseRecorder) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	return c, recorder
}

// TestIncidentBodyReadSiblingOwnership preserves cancellation/reset causes and closes each failed body once.
func TestIncidentBodyReadSiblingOwnership(t *testing.T) {
	for _, path := range incidentSiblingPaths() {
		for _, cause := range []struct {
			name string
			err  error
		}{{"reset", syscall.ECONNRESET}, {"cancelled", context.Canceled}} {
			for _, closeFailure := range []bool{false, true} {
				name := path.name + "/" + cause.name + "/close_ok"
				var closeErr error
				if closeFailure {
					name = path.name + "/" + cause.name + "/close_failed"
					closeErr = errors.New("synthetic body close failure")
				}
				t.Run(name, func(t *testing.T) {
					c, recorder := incidentSiblingContext()
					body := &incidentSiblingBody{reader: strings.NewReader(incidentBodyPrefix), readErr: cause.err, closeErr: closeErr}
					apiErr := path.run(c, &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: body})
					require.NotNil(t, apiErr)
					require.Equal(t, path.readCode, apiErr.Code)
					require.Equal(t, 500, apiErr.StatusCode)
					require.ErrorIs(t, apiErr.RawError, cause.err, "cleanup must preserve the original read failure")
					require.Equal(t, 1, body.closes)
					require.False(t, c.Writer.Written())
					require.Empty(t, recorder.Body.String())
				})
			}
		}
	}
}

// TestIncidentBodyReadSiblingCloseFailure preserves each entrypoint's established cleanup-error contract.
func TestIncidentBodyReadSiblingCloseFailure(t *testing.T) {
	for _, path := range incidentSiblingPaths() {
		t.Run(path.name, func(t *testing.T) {
			c, recorder := incidentSiblingContext()
			closeErr := errors.New("synthetic body close failure")
			body := &incidentSiblingBody{reader: strings.NewReader(path.successBody), closeErr: closeErr}
			apiErr := path.run(c, &http.Response{StatusCode: path.status, Header: http.Header{"Content-Type": {"application/json"}}, Body: body})
			require.Equal(t, 1, body.closes)
			if path.name == "deepseek_claude" {
				// Conversion historically continued after a successful read despite Close failure.
				require.Nil(t, apiErr)
				_, converted := c.Get(ctxkey.ConvertedResponse)
				require.True(t, converted)
				require.False(t, c.Writer.Written())
				return
			}
			if path.name == "deepseek_stream_json" {
				// A provider rejection must retain its original status and code.
				require.NotNil(t, apiErr)
				require.Equal(t, "synthetic_upstream_error", apiErr.Code)
				require.Equal(t, 502, apiErr.StatusCode)
				require.False(t, c.Writer.Written())
				return
			}
			require.NotNil(t, apiErr)
			require.Equal(t, "close_response_body_failed", apiErr.Code)
			require.Equal(t, 500, apiErr.StatusCode)
			require.ErrorIs(t, apiErr.RawError, closeErr)
			require.False(t, c.Writer.Written())
			require.Empty(t, recorder.Body.String())
			_, converted := c.Get(ctxkey.ConvertedResponse)
			require.False(t, converted)
		})
	}
}

// TestIncidentBodyReadSiblingSuccess retains normal buffered/conversion/provider-error behavior.
func TestIncidentBodyReadSiblingSuccess(t *testing.T) {
	for _, path := range incidentSiblingPaths() {
		t.Run(path.name, func(t *testing.T) {
			c, recorder := incidentSiblingContext()
			body := &incidentSiblingBody{reader: strings.NewReader(path.successBody)}
			apiErr := path.run(c, &http.Response{StatusCode: path.status, Header: http.Header{"Content-Type": {"application/json"}}, Body: body})
			require.Equal(t, 1, body.closes)
			switch path.name {
			case "deepseek_stream_json":
				require.NotNil(t, apiErr)
				require.Equal(t, 502, apiErr.StatusCode)
				require.Equal(t, "synthetic_upstream_error", apiErr.Code)
				require.Empty(t, recorder.Body.String())
			case "deepseek_claude":
				require.Nil(t, apiErr)
				value, ok := c.Get(ctxkey.ConvertedResponse)
				require.True(t, ok)
				converted, ok := value.(*http.Response)
				require.True(t, ok)
				convertedBody, err := io.ReadAll(converted.Body)
				require.NoError(t, err)
				require.NoError(t, converted.Body.Close())
				require.Contains(t, string(convertedBody), "synthetic local reply")
			default:
				require.Nil(t, apiErr)
				require.True(t, c.Writer.Written())
				require.NotEmpty(t, recorder.Body.String())
			}
		})
	}
}

// TestIncidentBodyResetClaudeConversion closes failed TCP responses without producing partial conversion.
func TestIncidentBodyResetClaudeConversion(t *testing.T) {
	for _, reused := range []bool{false, true} {
		name := "fresh"
		if reused {
			name = "reused"
		}
		for _, prefix := range []bool{false, true} {
			bodyName := "zero"
			if prefix {
				bodyName = "prefix"
			}
			t.Run(name+"/"+bodyName, func(t *testing.T) {
				f := newIncidentSocketFixture(t, "rst", prefix)
				resp := incidentResponse(t, f, reused, prefix)
				body := resp.Body.(*incidentObservedBody)
				c, recorder := incidentSiblingContext()
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
				c.Set(ctxkey.ClaudeMessagesConversion, true)
				usage, apiErr := (&deepseek.Adaptor{}).DoResponse(c, resp, &meta.Meta{ChannelType: channeltype.DeepSeek, ActualModelName: "deepseek-flash", Mode: relaymode.ChatCompletions})
				require.NotNil(t, apiErr)
				require.Equal(t, "read_response_body_failed", apiErr.Code)
				require.ErrorIs(t, apiErr.RawError, syscall.ECONNRESET)
				require.Nil(t, usage)
				require.EqualValues(t, 1, body.closes.Load())
				require.Empty(t, recorder.Body.String())
				_, converted := c.Get(ctxkey.ConvertedResponse)
				require.False(t, converted)
			})
		}
	}
}
