package palm

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/Laisky/one-api/relay/model"
	"github.com/stretchr/testify/require"
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
	}{{"chat", "Handler", func(resp *http.Response) (*model.ErrorWithStatusCode, *model.Usage) {
		return Handler(nil, resp, 0, "fixture")
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
				require.Contains(t, fmt.Sprintf("%+v", failure.RawError), "palm."+handler.frame)
			})
		}
	}
}
