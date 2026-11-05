package deepl

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/model"
)

// deeplTransportFailure is a typed external failure used to check error identity.
type deeplTransportFailure struct{}

// Error returns a stable transport error message.
func (*deeplTransportFailure) Error() string { return "synthetic transport failure" }

// deeplFailureBody records cleanup and supplies selected read and close failures.
type deeplFailureBody struct {
	reader            io.Reader
	readErr, closeErr error
	closes            int
}

// Read returns the selected read failure or reads the supplied body.
func (b *deeplFailureBody) Read(p []byte) (int, error) {
	if b.readErr != nil {
		return 0, b.readErr
	}
	return b.reader.Read(p)
}

// Close records body cleanup and returns the selected close failure.
func (b *deeplFailureBody) Close() error { b.closes++; return b.closeErr }

// TestDeepLResponseFailureStack checks non-streaming and streaming error contracts.
func TestDeepLResponseFailureStack(t *testing.T) {
	for _, handler := range []struct {
		name string
		run  func(*gin.Context, *http.Response, string) *model.ErrorWithStatusCode
	}{{"Handler", Handler}, {"StreamHandler", StreamHandler}} {
		for _, scenario := range []struct{ name, code string }{
			{"read", "read_response_body_failed"}, {"close", "close_response_body_failed"}, {"decode", "unmarshal_response_body_failed"},
		} {
			t.Run(handler.name+"/"+scenario.name, func(t *testing.T) {
				cause := &deeplTransportFailure{}
				body := &deeplFailureBody{reader: strings.NewReader("{")}
				switch scenario.name {
				case "read":
					body.readErr = cause
				case "close":
					body.closeErr = cause
				}
				failure := handler.run(nil, &http.Response{Body: body}, "fixture")
				require.NotNil(t, failure)
				require.Equal(t, http.StatusInternalServerError, failure.StatusCode)
				require.Equal(t, scenario.code, failure.Code)
				require.Equal(t, model.ErrorTypeOneAPI, failure.Type)
				if scenario.name == "decode" {
					var syntax *json.SyntaxError
					require.True(t, errors.As(failure.RawError, &syntax))
					require.Equal(t, "unexpected end of JSON input", failure.Message)
				} else {
					require.ErrorIs(t, failure.RawError, cause)
					var typed *deeplTransportFailure
					require.True(t, errors.As(failure.RawError, &typed))
					require.Same(t, cause, typed)
					require.Equal(t, cause.Error(), failure.Message)
				}
				if scenario.name == "read" {
					require.Zero(t, body.closes)
				} else {
					require.Equal(t, 1, body.closes)
				}
				require.Contains(t, fmt.Sprintf("%+v", failure.RawError), "deepl."+handler.name)
			})
		}
	}
}
