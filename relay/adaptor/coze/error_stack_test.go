package coze

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/model"
)

// responseFailure is a typed external error used to verify cause identity.
type responseFailure struct{}

// Error returns the fixture transport error message.
func (*responseFailure) Error() string { return "synthetic response failure" }

// failureBody supplies read and close errors and tracks cleanup.
type failureBody struct {
	reader            io.Reader
	readErr, closeErr error
	closes            int
}

// Read returns the selected read error or bytes from the supplied reader.
func (b *failureBody) Read(p []byte) (int, error) {
	if b.readErr != nil {
		return 0, b.readErr
	}
	return b.reader.Read(p)
}

// Close records the close call and returns the selected error.
func (b *failureBody) Close() error { b.closes++; return b.closeErr }

// TestHandlerErrorStacks verifies transport and decode errors without altering the API contract.
func TestHandlerErrorStacks(t *testing.T) {
	for _, scenario := range []struct{ name, code string }{
		{"read", "read_response_body_failed"}, {"close", "close_response_body_failed"}, {"decode", "unmarshal_response_body_failed"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			cause := &responseFailure{}
			body := &failureBody{reader: strings.NewReader("{")}
			switch scenario.name {
			case "read":
				body.readErr = cause
			case "close":
				body.closeErr = cause
			}
			failure, result := Handler(nil, &http.Response{Body: body}, 0, "fixture")
			require.Nil(t, result)
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
				var typed *responseFailure
				require.True(t, errors.As(failure.RawError, &typed))
				require.Same(t, cause, typed)
				require.Equal(t, cause.Error(), failure.Message)
			}
			if scenario.name == "read" {
				require.Zero(t, body.closes)
			} else {
				require.Equal(t, 1, body.closes)
			}
			require.Contains(t, fmt.Sprintf("%+v", failure.RawError), "coze.Handler")
		})
	}
}

// TestStreamCloseErrorStack preserves terminal SSE output and close-error identity.
func TestStreamCloseErrorStack(t *testing.T) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	cause := &responseFailure{}
	body := &failureBody{reader: strings.NewReader(""), closeErr: cause}
	resp := &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: body}
	failure, result := StreamHandler(c, resp)
	require.Nil(t, result)
	require.NotNil(t, failure)
	require.Equal(t, http.StatusInternalServerError, failure.StatusCode)
	require.Equal(t, "close_response_body_failed", failure.Code)
	require.Equal(t, model.ErrorTypeOneAPI, failure.Type)
	require.Equal(t, cause.Error(), failure.Message)
	require.ErrorIs(t, failure.RawError, cause)
	var typed *responseFailure
	require.True(t, errors.As(failure.RawError, &typed))
	require.Same(t, cause, typed)
	require.Equal(t, 1, body.closes)
	require.Contains(t, recorder.Body.String(), "data: [DONE]")
	require.Contains(t, fmt.Sprintf("%+v", failure.RawError), "coze.StreamHandler")
}
