package controller

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/zap"
	"github.com/Laisky/zap/zapcore"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	relaycontroller "github.com/Laisky/one-api/relay/controller"
)

const relayRawBodySecret = "SYNTHETIC_PRIVATE_RESPONSE_BODY"
const relayURLSecret = "SYNTHETIC_PRIVATE_QUERY_VALUE"

// TestRelayFailureExplicitRawBodyRedaction takes t and verifies only complete-body parser fallbacks are redacted in logs while provider reasons and API response bytes remain intact.
func TestRelayFailureExplicitRawBodyRedaction(t *testing.T) {
	for _, scenario := range []struct {
		name, body, message string
		raw                 bool
	}{
		{"invalid_json", "<html>" + relayRawBodySecret + "</html>", "failed to unmarshal response body: invalid character '<' looking for beginning of value\n\n<html>" + relayRawBodySecret + "</html>", true},
		{"unrecognized_json", `{"private_payload":"` + relayRawBodySecret + `"}`, `bad response [502] {"private_payload":"` + relayRawBodySecret + `"}`, true},
		{"structured_reason", `{"error":{"message":"synthetic provider rejected max_tokens=0","type":"upstream_error","code":"synthetic_parameter"}}`, "synthetic provider rejected max_tokens=0", false},
	} {
		for _, withContext := range []bool{false, true} {
			name := scenario.name + "/plain"
			if withContext {
				name = scenario.name + "/context"
			}
			t.Run(name, func(t *testing.T) {
				fixture := newSuspension5XXFixture(t, nil)
				response := &http.Response{StatusCode: 502, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(scenario.body))}
				parsed := relaycontroller.RelayErrorHandler(response)
				if withContext {
					c, _ := gin.CreateTestContext(httptest.NewRecorder())
					response.Body = io.NopCloser(strings.NewReader(scenario.body))
					parsed = relaycontroller.RelayErrorHandlerWithContext(c, response)
				}
				require.Equal(t, scenario.message, parsed.Message, "privacy policy must not alter the provider response returned to the API caller")
				before, err := json.Marshal(parsed)
				require.NoError(t, err)
				var public map[string]any
				require.NoError(t, json.Unmarshal(before, &public))
				require.Len(t, public, 5, "internal provenance must not add client-visible response fields")
				fixture.params.Err = *parsed
				processChannelRelayError(fixture.ctx, fixture.params)
				records := fixture.logs.FilterMessage("relay error").All()
				require.Len(t, records, 1)
				if scenario.raw {
					require.Equal(t, "[REDACTED]", records[0].ContextMap()["upstream_error"])
				} else {
					require.Equal(t, scenario.message, records[0].ContextMap()["upstream_error"])
				}
				for _, record := range fixture.logs.All() {
					buffer, err := zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig()).EncodeEntry(record.Entry, record.Context)
					require.NoError(t, err)
					rendered := buffer.String()
					buffer.Free()
					require.NotContains(t, rendered, relayRawBodySecret, "raw body must not escape through error/errorVerbose or another log field")
				}
				after, err := json.Marshal(parsed)
				require.NoError(t, err)
				require.Equal(t, before, after, "logging must not mutate API errors")
				if scenario.raw {
					require.Contains(t, string(after), relayRawBodySecret, "client API behavior remains unchanged")
				}
			})
		}
	}
}

// TestRelayFailureExplicitURLSanitization takes t and verifies request URL diagnostics retain endpoint context with visible markers for removed credential-bearing components.
func TestRelayFailureExplicitURLSanitization(t *testing.T) {
	for _, scenario := range []struct {
		name, input, want string
		changed           bool
	}{
		{"ordinary_path", "/v1/chat/completions", "/v1/chat/completions", false},
		{"query", "/v1/chat/completions?key=" + relayURLSecret + "&mode=1", "/v1/chat/completions", true},
		{"userinfo_query_fragment", "https://fixture-user:" + relayURLSecret + "@provider.example.invalid/v1/chat?key=" + relayURLSecret + "#" + relayURLSecret, "https://provider.example.invalid/v1/chat", true},
		{"invalid_marker", "[REDACTED]", "[REDACTED]", true},
		{"invalid", "/v1/%zz?key=" + relayURLSecret, "[REDACTED]", true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			fixture := newSuspension5XXFixture(t, nil)
			fixture.params.RequestURL = scenario.input
			processChannelRelayError(fixture.ctx, fixture.params)
			records := fixture.logs.FilterMessage("relay error").All()
			require.Len(t, records, 1)
			fields := records[0].ContextMap()
			require.Equal(t, scenario.want, fields["request_url"])
			if scenario.changed {
				require.Equal(t, true, fields["request_url_redacted"])
			} else {
				require.NotContains(t, fields, "request_url_redacted")
			}
			require.Equal(t, fixture.params.OriginalModel, fields["origin_model"])
			require.Equal(t, fixture.params.ActualModel, fields["actual_model"])
			for _, record := range fixture.logs.All() {
				buffer, err := zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig()).EncodeEntry(record.Entry, record.Context)
				require.NoError(t, err)
				rendered := buffer.String()
				buffer.Free()
				require.NotContains(t, rendered, relayURLSecret)
			}
			require.Equal(t, scenario.input, fixture.params.RequestURL, "diagnostic policy must not mutate the request snapshot")
		})
	}
}

// TestRelayFailureNumericJSONErrorRedaction takes t and verifies raw numeric decoder values never escape serialized relay diagnostics while the client error remains unchanged.
func TestRelayFailureNumericJSONErrorRedaction(t *testing.T) {
	const literal = "918273645546372819e9999"
	for _, status := range []int{400, 502} {
		for _, withContext := range []bool{false, true} {
			t.Run(fmt.Sprintf("status_%d/context_%t", status, withContext), func(t *testing.T) {
				fixture := newSuspension5XXFixture(t, nil)
				body := `{"error":{"code":` + literal + `}}`
				response := &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}
				parsed := relaycontroller.RelayErrorHandler(response)
				if withContext {
					c, _ := gin.CreateTestContext(httptest.NewRecorder())
					response.Body = io.NopCloser(strings.NewReader(body))
					parsed = relaycontroller.RelayErrorHandlerWithContext(c, response)
				}
				require.Contains(t, parsed.Message, literal)
				require.Contains(t, parsed.RawError.Error(), literal)
				parsed.RawError = errors.Wrap(parsed.RawError, "decode synthetic relay body")
				before, err := json.Marshal(parsed)
				require.NoError(t, err)
				fixture.params.Err = *parsed
				processChannelRelayError(fixture.ctx, fixture.params)
				records := fixture.logs.FilterMessage("relay error").All()
				require.Len(t, records, 1)
				require.Equal(t, "[REDACTED]", records[0].ContextMap()["upstream_error"])
				for _, record := range fixture.logs.All() {
					buffer, err := zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig()).EncodeEntry(record.Entry, record.Context)
					require.NoError(t, err)
					rendered := buffer.String()
					buffer.Free()
					require.NotContains(t, rendered, literal, "error and errorVerbose must not reintroduce a raw numeric payload")
				}
				after, err := json.Marshal(parsed)
				require.NoError(t, err)
				require.Equal(t, before, after)
				require.Contains(t, parsed.RawError.Error(), literal, "logging must not mutate the original error cause")
			})
		}
	}
}
