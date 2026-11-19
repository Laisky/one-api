package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	gmw "github.com/Laisky/gin-middlewares/v7"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/Laisky/zap"
	"github.com/Laisky/zap/zapcore"
	"github.com/Laisky/zap/zaptest/observer"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	relaymodel "github.com/Laisky/one-api/relay/model"
)

// providerIdentifierLogFieldMap encodes fields the way a structured sink does.
//
// Parameters:
//   - fields: the zap fields to encode.
//
// Returns:
//   - map[string]any: the encoded key/value pairs.
func providerIdentifierLogFieldMap(fields []zap.Field) map[string]any {
	encoder := zapcore.NewMapObjectEncoder()
	for _, field := range fields {
		field.AddTo(encoder)
	}
	return encoder.Fields
}

// TestResponseAPIErrorLogFieldsRedactsProviderIdentifiers verifies that the
// Response API DoResponse DEBUG trace logs vocabulary-shaped provider codes
// verbatim and redacts secret-shaped code, type and param values, and that a
// message flagged as embedding a raw body is redacted too.
func TestResponseAPIErrorLogFieldsRedactsProviderIdentifiers(t *testing.T) {
	require.Equal(t, map[string]any{"has_usage": true, "has_error": false},
		providerIdentifierLogFieldMap(responseAPIErrorLogFields(true, nil)))

	vocabulary := relaymodel.NewUpstreamError(http.StatusBadRequest, "maximum context length exceeded",
		relaymodel.ErrorType("invalid_request_error"), "context_length_exceeded", relaymodel.WithUpstreamErrorParam("messages"))
	fields := providerIdentifierLogFieldMap(responseAPIErrorLogFields(false, vocabulary))
	require.Equal(t, "invalid_request_error", fields["error_type"])
	require.Equal(t, "context_length_exceeded", fields["error_code"])
	require.Equal(t, "messages", fields["error_param"])
	require.Equal(t, "maximum context length exceeded", fields["error_message"])
	require.Equal(t, int64(http.StatusBadRequest), fields["status_code"])

	secret := relaymodel.NewUpstreamError(http.StatusBadGateway, "synthetic raw body",
		relaymodel.ErrorType("eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiJhY2N0In0.c2ln"), "sk-proj-AbCdEf0123456789GhIjKlMn",
		relaymodel.WithUpstreamErrorParam("someone@example.com"))
	secret.MessageContainsRawBody = true
	fields = providerIdentifierLogFieldMap(responseAPIErrorLogFields(false, secret))
	for _, key := range []string{"error_type", "error_code", "error_param", "error_message"} {
		require.Equal(t, "[REDACTED]", fields[key], key)
	}
	for _, key := range []string{"error_type", "error_code", "error_param"} {
		require.Equal(t, true, fields[key+"_redacted"], key)
	}
	require.Equal(t, "sk-proj-AbCdEf0123456789GhIjKlMn", secret.Code, "logging must not mutate the client-visible error")
}

// TestClaudeCountTokensRejectionLogRedactsProviderErrorType drives
// writeClaudeCountTokensUpstreamReply with native Anthropic error envelopes and
// verifies that a secret-shaped error type never reaches the WARN line while a
// vocabulary type is logged verbatim; the client still receives the body.
func TestClaudeCountTokensRejectionLogRedactsProviderErrorType(t *testing.T) {
	for _, scenario := range []struct {
		name, errorType string
		redacted        bool
	}{
		{"vocabulary", "invalid_request_error", false},
		{"credential", "sk-ant-api03-AbCdEf0123456789GhIjKlMn", true},
		{"free_text", "rejected for someone@example.com", true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages/count_tokens", http.NoBody)
			core, observed := observer.New(zapcore.DebugLevel)
			lg, err := glog.New(glog.WithName("count-tokens-redaction"), glog.WithLevel(glog.LevelDebug),
				glog.WithZapOptions(zap.WrapCore(func(zapcore.Core) zapcore.Core { return core })))
			require.NoError(t, err)
			gmw.SetLogger(c, lg)
			payload := []byte(`{"type":"error","error":{"type":"` + scenario.errorType + `","message":"rejected"}}`)
			resp := &http.Response{StatusCode: http.StatusBadRequest, Header: make(http.Header)}

			status, written := writeClaudeCountTokensUpstreamReply(c, resp, payload)
			require.True(t, written)
			require.Equal(t, http.StatusBadRequest, status)
			require.Equal(t, string(payload), recorder.Body.String(), "the client receives the native provider error unchanged")
			records := observed.FilterMessage("upstream rejected claude count_tokens request").All()
			require.Len(t, records, 1)
			fields := records[0].ContextMap()
			if scenario.redacted {
				require.Equal(t, "[REDACTED]", fields["upstream_error_type"])
				require.Equal(t, true, fields["upstream_error_type_redacted"])
			} else {
				require.Equal(t, scenario.errorType, fields["upstream_error_type"])
				require.NotContains(t, fields, "upstream_error_type_redacted")
			}
			for _, record := range observed.All() {
				buffer, err := zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig()).EncodeEntry(record.Entry, record.Context)
				require.NoError(t, err)
				rendered := buffer.String()
				buffer.Free()
				if scenario.redacted {
					require.False(t, strings.Contains(rendered, scenario.errorType), "provider error type leaked: %s", rendered)
				}
			}
		})
	}
}
