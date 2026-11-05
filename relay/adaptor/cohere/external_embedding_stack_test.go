package cohere

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/meta"
)

// externalFailureWriter models a downstream delivery error while retaining Gin methods.
type externalFailureWriter struct {
	gin.ResponseWriter
	err error
}

// Write returns the configured client-write failure.
func (w *externalFailureWriter) Write([]byte) (int, error) { return 0, w.err }

// TestExternalEmbeddingErrorsStack preserves error identity and billing on response failures.
func TestExternalEmbeddingErrorsStack(t *testing.T) {
	for _, tc := range []struct {
		name, code string
		status     int
	}{
		{"read", "read_embedding_response", http.StatusBadGateway},
		{"decode", "invalid_embedding_response", http.StatusBadGateway},
		{"write", "write_embedding_response", http.StatusInternalServerError},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/embeddings", nil)
			cause := &externalFailure{}
			payload := `{"embeddings":{"float":[[1]]},"meta":{"billed_units":{"input_tokens":7}}}`
			if tc.name == "decode" {
				payload = "{"
			}
			body := &externalFailureBody{reader: strings.NewReader(payload)}
			if tc.name == "read" {
				body.readErr = cause
			}
			if tc.name == "write" {
				c.Writer = &externalFailureWriter{ResponseWriter: c.Writer, err: cause}
			}
			usage, failure := embedResponse(c, &http.Response{StatusCode: 200, Header: make(http.Header), Body: body}, &meta.Meta{ActualModelName: "fixture"})
			require.NotNil(t, failure)
			require.Equal(t, tc.status, failure.StatusCode)
			require.Equal(t, tc.code, failure.Code)
			require.Equal(t, 1, body.closes)
			if tc.name == "decode" {
				var syntax *json.SyntaxError
				require.ErrorAs(t, failure.RawError, &syntax)
				require.Equal(t, "unexpected end of JSON input", failure.Message)
			} else {
				require.ErrorIs(t, failure.RawError, cause)
				require.Equal(t, cause.Error(), failure.Message)
			}
			if tc.name == "write" {
				require.NotNil(t, usage)
				require.Equal(t, 7, usage.PromptTokens)
			} else {
				require.Nil(t, usage)
			}
			require.Contains(t, fmt.Sprintf("%+v", failure.RawError), "cohere.embedResponse")
		})
	}
}

// TestExternalRerankWriteErrorStack retains the measured receipt when delivery fails.
func TestExternalRerankWriteErrorStack(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/rerank", nil)
	cause := &externalFailure{}
	c.Writer = &externalFailureWriter{ResponseWriter: c.Writer, err: cause}
	body := &externalFailureBody{reader: strings.NewReader(`{"results":[],"meta":{"billed_units":{"search_units":3},"tokens":{"input_tokens":11}}}`)}
	failure, usage := RerankHandler(c, &http.Response{StatusCode: 200, Body: body}, &meta.Meta{ActualModelName: "fixture"})
	require.NotNil(t, failure)
	require.Equal(t, http.StatusInternalServerError, failure.StatusCode)
	require.Equal(t, "write_response_body_failed", failure.Code)
	require.Equal(t, cause.Error(), failure.Message)
	require.ErrorIs(t, failure.RawError, cause)
	require.NotNil(t, usage)
	require.NotNil(t, usage.BilledSearchUnits)
	require.EqualValues(t, 3, *usage.BilledSearchUnits)
	require.Equal(t, 11, usage.PromptTokens)
	require.Equal(t, 1, body.closes)
	require.Contains(t, fmt.Sprintf("%+v", failure.RawError), "cohere.RerankHandler")
}
