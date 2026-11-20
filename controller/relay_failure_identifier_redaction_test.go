package controller

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/Laisky/zap"
	"github.com/Laisky/zap/zapcore"
	"github.com/Laisky/zap/zaptest/observer"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/common/graceful"
	relaycontroller "github.com/Laisky/one-api/relay/controller"
	relaymodel "github.com/Laisky/one-api/relay/model"
)

// providerIdentifierSecretScenario is a provider error body whose code, type or
// param carries a value that must never reach operator logs.
type providerIdentifierSecretScenario struct {
	name string
	// body is the upstream error body the relay decodes.
	body string
	// field is the log key that carries the secret ("error_code", ...).
	field string
	// needles are the substrings that must not appear in any rendered entry.
	needles []string
	// code, when non-nil, replaces the decoded code the way an adaptor that
	// builds the error from structured provider fields would set it; the relay
	// body decoder itself drops non-scalar codes.
	code any
}

// providerIdentifierSecretScenarios returns provider bodies carrying a free-form
// value, an e-mail address, or an identifier-shaped credential or opaque
// identifier in code, type or param.
//
// Returns:
//   - []providerIdentifierSecretScenario: one scenario per leak shape.
func providerIdentifierSecretScenarios() []providerIdentifierSecretScenario {
	envelope := func(member, value string) string {
		return `{"error":{"message":"synthetic provider failure","` + member + `":` + value + `}}`
	}
	return []providerIdentifierSecretScenario{
		{name: "email_param", body: envelope("param", `"someone@example.com"`), field: "error_param", needles: []string{"someone@example.com"}},
		{name: "free_text_type", body: envelope("type", `"rate limit for internal-account-123"`), field: "error_type", needles: []string{"internal-account-123"}},
		{name: "openai_key_code", body: envelope("code", `"sk-proj-AbCdEf0123456789GhIjKlMn"`), field: "error_code", needles: []string{"AbCdEf0123456789GhIjKlMn"}},
		{name: "underscore_key_code", body: envelope("code", `"sk_live_SECRETSECRETSECRET"`), field: "error_code", needles: []string{"SECRETSECRETSECRET"}},
		{name: "jwt_type", body: envelope("type", `"eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiJhY2N0In0.c2lnbmF0dXJlLXNpZ25hdHVyZQ"`), field: "error_type", needles: []string{"eyJhbGciOiJIUzI1NiJ9"}},
		{name: "hex_token_param", body: envelope("param", `"9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08"`), field: "error_param", needles: []string{"9f86d081884c7d659a2feaa0c55ad015"}},
		{name: "aws_key_id_code", body: envelope("code", `"AKIAIOSFODNN7EXAMPLE"`), field: "error_code", needles: []string{"AKIAIOSFODNN7EXAMPLE"}},
		{name: "account_id_type", body: envelope("type", `"acct_1Nv0FGQ9RKHgCVdK"`), field: "error_type", needles: []string{"1Nv0FGQ9RKHgCVdK"}},
		{name: "uuid_param", body: envelope("param", `"3f2504e0-4f89-41d3-9a0c-0305e82c3301"`), field: "error_param", needles: []string{"3f2504e0-4f89-41d3-9a0c-0305e82c3301"}},
		{name: "account_number_code", body: envelope("code", `"123456789012"`), field: "error_code", needles: []string{"123456789012"}},
		{name: "card_number_numeric_code", body: envelope("code", `4111111111111111`), field: "error_code", needles: []string{"4111111111111111", "4.111111111111111e+15"}},
		{name: "object_code", body: envelope("type", `"upstream_error"`), field: "error_code", needles: []string{"internal-account-123"},
			code: map[string]any{"account": "internal-account-123"}},
	}
}

// renderObservedRelayLogs encodes every observed entry the way a JSON sink does,
// so assertions see the error, errorVerbose and nested values too.
//
// Parameters:
//   - t: the running test.
//   - logs: the observed entries.
//
// Returns:
//   - []string: one rendered JSON line per entry.
func renderObservedRelayLogs(t *testing.T, logs *observer.ObservedLogs) []string {
	t.Helper()
	rendered := make([]string, 0, logs.Len())
	for _, record := range logs.All() {
		buffer, err := zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig()).EncodeEntry(record.Entry, record.Context)
		require.NoError(t, err)
		rendered = append(rendered, buffer.String())
		buffer.Free()
	}
	return rendered
}

// TestRelayFailureProviderIdentifierRedaction takes t and verifies that free-form
// or secret-looking provider code, type and param values never reach any relay
// failure log entry, across the 400, 429 and 5xx branches of
// processChannelRelayError, and that the redacted key is marked.
func TestRelayFailureProviderIdentifierRedaction(t *testing.T) {
	for _, scenario := range providerIdentifierSecretScenarios() {
		for _, status := range []int{http.StatusBadRequest, http.StatusTooManyRequests, http.StatusBadGateway} {
			t.Run(fmt.Sprintf("%s/%d", scenario.name, status), func(t *testing.T) {
				fixture := newSuspension5XXFixture(t, nil)
				parsed := relaycontroller.RelayErrorHandler(&http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(scenario.body))})
				require.Equal(t, "synthetic provider failure", parsed.Message, "the body must decode as a structured provider error")
				if scenario.code != nil {
					parsed.Code = scenario.code
				}
				fixture.params.Err = *parsed
				processChannelRelayError(fixture.ctx, fixture.params)
				rendered := renderObservedRelayLogs(t, fixture.logs)
				require.NotEmpty(t, rendered)
				for _, line := range rendered {
					for _, needle := range scenario.needles {
						require.NotContains(t, line, needle, "a provider-controlled %s value leaked into a log entry", scenario.field)
					}
				}
				records := fixture.logs.FilterMessage("relay error").All()
				require.Len(t, records, 1)
				fields := records[0].ContextMap()
				require.Equal(t, "[REDACTED]", fields[scenario.field])
				require.Equal(t, true, fields[scenario.field+"_redacted"])
			})
		}
	}
}

// TestRelayFailureProviderVocabularyLoggedVerbatim takes t and verifies that the
// provider error vocabulary operators search for (snake_case and CamelCase codes,
// numeric codes, parameter paths) still reaches the relay failure log unchanged
// and without a redaction marker.
func TestRelayFailureProviderVocabularyLoggedVerbatim(t *testing.T) {
	for _, scenario := range []struct {
		name   string
		status int
		body   string
		want   map[string]string
	}{
		{"openai_rate_limit", http.StatusTooManyRequests, `{"error":{"message":"Rate limit reached","type":"requests","param":null,"code":"rate_limit_exceeded"}}`,
			map[string]string{"error_code": "rate_limit_exceeded", "error_type": "requests"}},
		{"openai_insufficient_quota", http.StatusTooManyRequests, `{"error":{"message":"You exceeded your current quota","type":"insufficient_quota","code":"insufficient_quota"}}`,
			map[string]string{"error_code": "insufficient_quota", "error_type": "insufficient_quota"}},
		{"openai_context_length", http.StatusBadRequest, `{"error":{"message":"maximum context length exceeded","type":"invalid_request_error","param":"messages","code":"context_length_exceeded"}}`,
			map[string]string{"error_code": "context_length_exceeded", "error_type": "invalid_request_error", "error_param": "messages"}},
		{"ark_account_quota", http.StatusTooManyRequests, `{"error":{"message":"account quota exceeded","type":"TooManyRequests","code":"AccountQuotaExceeded"}}`,
			map[string]string{"error_code": "AccountQuotaExceeded", "error_type": "TooManyRequests"}},
		{"ark_tpm_limit", http.StatusTooManyRequests, `{"error":{"message":"tpm limit","code":"ModelAccountTpmRateLimitExceeded"}}`,
			map[string]string{"error_code": "ModelAccountTpmRateLimitExceeded"}},
		{"bedrock_throughput", http.StatusBadGateway, `{"error":{"message":"throughput exceeded","type":"ProvisionedThroughputExceededException","code":"throttled"}}`,
			map[string]string{"error_code": "throttled", "error_type": "ProvisionedThroughputExceededException"}},
		{"numeric_429", http.StatusTooManyRequests, `{"error":{"message":"too many requests","code":429}}`,
			map[string]string{"error_code": "429"}},
		{"baidu_numeric", http.StatusBadGateway, `{"error":{"message":"qps limit","code":336003}}`,
			map[string]string{"error_code": "336003"}},
		{"zhipu_numeric_string", http.StatusTooManyRequests, `{"error":{"message":"concurrency limit","code":"1302"}}`,
			map[string]string{"error_code": "1302"}},
		{"param_path", http.StatusBadRequest, `{"error":{"message":"invalid image url","type":"invalid_request_error","param":"messages[0].content[1].image_url.url","code":"invalid_image_url"}}`,
			map[string]string{"error_code": "invalid_image_url", "error_param": "messages[0].content[1].image_url.url"}},
		{"dated_model_param", http.StatusBadRequest, `{"error":{"message":"unknown model","param":"claude-3-5-sonnet-20241022","code":"model_not_found"}}`,
			map[string]string{"error_code": "model_not_found", "error_param": "claude-3-5-sonnet-20241022"}},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			fixture := newSuspension5XXFixture(t, nil)
			parsed := relaycontroller.RelayErrorHandler(&http.Response{StatusCode: scenario.status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(scenario.body))})
			fixture.params.Err = *parsed
			processChannelRelayError(fixture.ctx, fixture.params)
			records := fixture.logs.FilterMessage("relay error").All()
			require.Len(t, records, 1)
			fields := records[0].ContextMap()
			for key, want := range scenario.want {
				require.Equal(t, want, fields[key], "operators rely on %s being logged verbatim", key)
				require.NotContains(t, fields, key+"_redacted")
			}
		})
	}
}

// TestRelayRetryDecisionRedactsProviderIdentifiers takes t and drives Relay()
// with an upstream 400 whose code, type and param carry secrets and whose
// message is flagged as embedding a raw body, so the DEBUG "non-retry relay
// decision details" line, the WARN retry-skip line and the asynchronous relay
// failure records are all checked through the real path.
func TestRelayRetryDecisionRedactsProviderIdentifiers(t *testing.T) {
	gin.SetMode(gin.TestMode)
	originalRetryTimes, originalDebug := config.RetryTimes, config.DebugEnabled
	config.RetryTimes, config.DebugEnabled = 3, true
	t.Cleanup(func() { config.RetryTimes, config.DebugEnabled = originalRetryTimes, originalDebug })

	const secretCode = "sk-proj-AbCdEf0123456789GhIjKlMn"
	const secretType = "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiJhY2N0In0.c2lnbmF0dXJlLXNpZ25hdHVyZQ"
	const secretParam = "someone@example.com"
	const secretRawBody = "SYNTHETIC_PRIVATE_RAW_BODY"
	oldHelper := relayHelperForTest
	relayHelperForTest = func(*gin.Context, int) *relaymodel.ErrorWithStatusCode {
		failure := relaymodel.NewUpstreamError(http.StatusBadRequest, "synthetic fallback "+secretRawBody,
			relaymodel.ErrorType(secretType), secretCode, relaymodel.WithUpstreamErrorParam(secretParam))
		// MessageContainsRawBody covers the message only; a producer keeps the
		// body out of RawError, as the explicit_raw_body_flag fixture does.
		failure.MessageContainsRawBody = true
		failure.RawError = errors.New("synthetic provider rejected the request")
		return failure
	}
	t.Cleanup(func() { relayHelperForTest = oldHelper })

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	core, observed := observer.New(zapcore.DebugLevel)
	requestLogger, err := glog.New(glog.WithName("relay-identifier-redaction"), glog.WithLevel(glog.LevelDebug),
		glog.WithZapOptions(zap.WrapCore(func(zapcore.Core) zapcore.Core { return core })))
	require.NoError(t, errors.Wrap(err, "create observer logger"))
	gmw.SetLogger(c, requestLogger)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", http.NoBody)
	c.Set(ctxkey.Id, 1)
	c.Set(ctxkey.TokenId, 73)
	c.Set(ctxkey.ChannelId, 5)
	c.Set(ctxkey.ChannelName, "synthetic")
	c.Set(ctxkey.Group, "default")
	c.Set(ctxkey.RequestModel, "gpt-4o-mini")
	c.Set(ctxkey.RequestId, "synthetic-identifier-redaction")

	Relay(c)
	drainCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, errors.Wrap(graceful.Drain(drainCtx), "drain async error processing"))

	decision := observed.FilterMessage("non-retry relay decision details").All()
	require.Len(t, decision, 1)
	require.Equal(t, true, decision[0].ContextMap()["error_code_redacted"])
	require.Equal(t, true, decision[0].ContextMap()["error_type_redacted"])
	skipped := observed.FilterMessage("relay retry skipped after failure").All()
	require.Len(t, skipped, 1)
	require.Equal(t, true, skipped[0].ContextMap()["error_param_redacted"])
	for _, line := range renderObservedRelayLogs(t, observed) {
		for _, needle := range []string{"AbCdEf0123456789GhIjKlMn", "eyJhbGciOiJIUzI1NiJ9", secretParam, secretRawBody} {
			require.NotContains(t, line, needle)
		}
	}
	require.Contains(t, recorder.Body.String(), secretCode, "the caller still receives the provider error unchanged")
}

// observedGinLogger installs an observed request-scoped DEBUG logger on c.
//
// Parameters:
//   - t: the running test.
//   - c: the request context that receives the logger.
//
// Returns:
//   - *observer.ObservedLogs: the captured entries.
func observedGinLogger(t *testing.T, c *gin.Context) *observer.ObservedLogs {
	t.Helper()
	core, observed := observer.New(zapcore.DebugLevel)
	lg, err := glog.New(glog.WithName("relay-identifier-redaction"), glog.WithLevel(glog.LevelDebug),
		glog.WithZapOptions(zap.WrapCore(func(zapcore.Core) zapcore.Core { return core })))
	require.NoError(t, errors.Wrap(err, "create observer logger"))
	gmw.SetLogger(c, lg)
	return observed
}

// TestRelayDebugDecisionLinesRedactProviderCodes takes t and verifies the two
// remaining DEBUG decision lines that carry a relay error code: the delivery
// re-attribution line logs a vocabulary adaptor code verbatim and redacts a
// secret-shaped one, and the paid video rejection verdict line redacts a
// secret-shaped provider code.
func TestRelayDebugDecisionLinesRedactProviderCodes(t *testing.T) {
	const secretCode = "sk-proj-AbCdEf0123456789GhIjKlMn"
	for _, scenario := range []struct {
		code     string
		redacted bool
	}{{"claude_response_failed", false}, {secretCode, true}} {
		t.Run("delivery/"+scenario.code[:8], func(t *testing.T) {
			c, tracker, release, cause := trackedDeliveryContext(t, context.Background(), "text/event-stream")
			defer release()
			observed := observedGinLogger(t, c)
			failure := &relaymodel.ErrorWithStatusCode{StatusCode: http.StatusBadGateway, Error: relaymodel.Error{
				Type: "upstream_error", Code: scenario.code, Message: "Claude response failed",
				RawError: errors.Wrap(cause, "write Claude response")}}
			require.True(t, relaymodel.IsDeliveryFailure(attributeDeliveryFailure(c, tracker, failure)))
			records := observed.FilterMessage("relay failure caused by a downstream write re-attributed to delivery").All()
			require.Len(t, records, 1)
			fields := records[0].ContextMap()
			if scenario.redacted {
				require.Equal(t, "[REDACTED]", fields["adaptor_error_code"])
				require.Equal(t, true, fields["adaptor_error_code_redacted"])
			} else {
				require.Equal(t, scenario.code, fields["adaptor_error_code"])
				require.NotContains(t, fields, "adaptor_error_code_redacted")
			}
		})
	}
	t.Run("video_verdict", func(t *testing.T) {
		gin.SetMode(gin.TestMode)
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", http.NoBody)
		c.Set(ctxkey.ChannelId, 5)
		observed := observedGinLogger(t, c)
		recordVideoCreationVerdict(c, relaymodel.NewUpstreamError(http.StatusBadRequest, "rejected",
			relaymodel.ErrorType("invalid_request_error"), secretCode))
		records := observed.FilterMessage("provider rejected paid video creation before admission; replay on another channel is safe").All()
		require.Len(t, records, 1)
		require.Equal(t, true, records[0].ContextMap()["error_code_redacted"])
		require.Equal(t, "invalid_request_error", records[0].ContextMap()["error_type"])
		for _, line := range renderObservedRelayLogs(t, observed) {
			require.NotContains(t, line, "AbCdEf0123456789GhIjKlMn")
		}
	})
}
