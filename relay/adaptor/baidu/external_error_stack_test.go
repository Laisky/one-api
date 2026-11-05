package baidu

import (
	"encoding/json"
	"fmt"
	"github.com/Laisky/one-api/relay/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// externalFailure is a typed transport error used to check identity.
type externalFailure struct{}

// Error returns the transport error's stable message.
func (*externalFailure) Error() string { return "synthetic external failure" }

// externalFailureBody exposes selected I/O failures and tracks closes.
type externalFailureBody struct {
	reader            io.Reader
	readErr, closeErr error
	closes            int
}

// Read returns a selected read failure or bytes from the source.
func (b *externalFailureBody) Read(p []byte) (int, error) {
	if b.readErr != nil {
		return 0, b.readErr
	}
	return b.reader.Read(p)
}

// Close records closure and returns the selected close failure.
func (b *externalFailureBody) Close() error { b.closes++; return b.closeErr }

// TestExternalHandlerErrorStack verifies protocol fields and causes for buffered failures.
func TestExternalHandlerErrorStack(t *testing.T) {
	for _, handler := range []struct {
		name, frame string
		run         func(*http.Response) (*model.ErrorWithStatusCode, *model.Usage)
	}{{"chat", "Handler", func(resp *http.Response) (*model.ErrorWithStatusCode, *model.Usage) { return Handler(nil, resp) }}, {"embedding", "EmbeddingHandler", func(resp *http.Response) (*model.ErrorWithStatusCode, *model.Usage) {
		return EmbeddingHandler(nil, resp)
	}}} {
		for _, tc := range []struct{ name, code string }{{"read", "read_response_body_failed"}, {"close", "close_response_body_failed"}, {"decode", "unmarshal_response_body_failed"}} {
			t.Run(handler.name+"/"+tc.name, func(t *testing.T) {
				cause := &externalFailure{}
				body := &externalFailureBody{reader: strings.NewReader("{")}
				switch tc.name {
				case "read":
					body.readErr = cause
				case "close":
					body.closeErr = cause
				}
				failure, usage := handler.run(&http.Response{Body: body})
				require.Nil(t, usage)
				require.NotNil(t, failure)
				require.Equal(t, 500, failure.StatusCode)
				require.Equal(t, model.ErrorTypeOneAPI, failure.Type)
				require.Equal(t, tc.code, failure.Code)
				if tc.name == "decode" {
					var syntax *json.SyntaxError
					require.ErrorAs(t, failure.RawError, &syntax)
					require.Equal(t, "unexpected end of JSON input", failure.Message)
				} else {
					require.ErrorIs(t, failure.RawError, cause)
					var typed *externalFailure
					require.ErrorAs(t, failure.RawError, &typed)
					require.Same(t, cause, typed)
					require.Equal(t, cause.Error(), failure.Message)
				}
				if tc.name == "read" {
					require.Zero(t, body.closes)
				} else {
					require.Equal(t, 1, body.closes)
				}
				require.Contains(t, fmt.Sprintf("%+v", failure.RawError), "baidu."+handler.frame)
			})
		}
	}
}

// TestExternalStreamCloseStack retains the existing emitted terminal event and close failure.
func TestExternalStreamCloseStack(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	cause := &externalFailure{}
	body := &externalFailureBody{reader: strings.NewReader(""), closeErr: cause}
	failure, usage := StreamHandler(c, &http.Response{StatusCode: 200, Body: body})
	require.Nil(t, usage)
	require.NotNil(t, failure)
	require.Equal(t, 500, failure.StatusCode)
	require.Equal(t, "close_response_body_failed", failure.Code)
	require.Equal(t, cause.Error(), failure.Message)
	require.ErrorIs(t, failure.RawError, cause)
	require.Equal(t, 1, body.closes)
	require.Contains(t, w.Body.String(), "data: [DONE]")
	require.Contains(t, fmt.Sprintf("%+v", failure.RawError), "baidu.StreamHandler")
}
