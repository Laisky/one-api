package controller

import (
	"bytes"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Laisky/errors/v2"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/protocol/eventstream"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/model"
	awsadaptor "github.com/Laisky/one-api/relay/adaptor/aws"
	"github.com/Laisky/one-api/relay/apitype"
	"github.com/Laisky/one-api/relay/meta"
	relaymodel "github.com/Laisky/one-api/relay/model"
)

const awsSettlementReceipt = `{"input_tokens":21,"output_tokens":50,"cache_read_input_tokens":1000,"cache_creation_input_tokens":300,"cache_creation":{"ephemeral_5m_input_tokens":100,"ephemeral_1h_input_tokens":200}}`

// awsSettlementTransport provides HTTP receipts to the real AWS SDK and counts
// invocations without using any external endpoint or paid inference.
type awsSettlementTransport struct {
	body   []byte
	stream bool
	calls  atomic.Int64
}

// RoundTrip returns the test receipt in the selected AWS wire format.
func (r *awsSettlementTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	r.calls.Add(1)
	contentType := "application/json"
	if r.stream {
		contentType = "application/vnd.amazon.eventstream"
	}
	return &http.Response{StatusCode: http.StatusOK, Request: request,
		Header: http.Header{"Content-Type": {contentType}, "X-Amzn-Bedrock-Content-Type": {"application/json"}},
		Body:   io.NopCloser(bytes.NewReader(r.body))}, nil
}

// awsSettlementFrames encodes genuine AWS EventStream framing and optionally
// truncates after the final usage receipt, before message_stop.
func awsSettlementFrames(t *testing.T, truncated bool) []byte {
	t.Helper()
	events := []string{
		`{"type":"message_start","message":{"id":"msg_settlement","type":"message","role":"assistant","content":[],"usage":` + awsSettlementReceipt + `}}`,
		`{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":` + awsSettlementReceipt + `}`,
	}
	if !truncated {
		events = append(events, `{"type":"message_stop"}`)
	}
	var out bytes.Buffer
	for _, event := range events {
		payload, err := json.Marshal(map[string][]byte{"bytes": []byte(event)})
		require.NoError(t, err)
		var headers eventstream.Headers
		headers.Set(":message-type", eventstream.StringValue("event"))
		headers.Set(":event-type", eventstream.StringValue("chunk"))
		headers.Set(":content-type", eventstream.StringValue("application/json"))
		require.NoError(t, eventstream.NewEncoder().Encode(&out, eventstream.Message{Headers: headers, Payload: payload}))
	}
	return out.Bytes()
}

// awsSettlementFailedWriter injects a failure before any response is committed.
type awsSettlementFailedWriter struct{ gin.ResponseWriter }

// Write simulates a disconnected client while leaving Written false.
func (w *awsSettlementFailedWriter) Write(_ []byte) (int, error) {
	return 0, errors.New("synthetic settlement delivery failure")
}

// WriteString uses the same failed-delivery path for string renderers.
func (w *awsSettlementFailedWriter) WriteString(p string) (int, error) { return w.Write([]byte(p)) }

// TestAWSClaudeInvokeSettlesUserAndTokenExactlyOnce exercises real admission,
// SDK receipt decoding, five-bucket pricing, database settlement, and the actual
// replay/refund guard. It is serial because it uses the shared DB fixtures.
func TestAWSClaudeInvokeSettlesUserAndTokenExactlyOnce(t *testing.T) {
	ensureResponseFallbackFixtures(t)
	oldRedis := common.IsRedisEnabled()
	common.SetRedisEnabled(false)
	t.Cleanup(func() { common.SetRedisEnabled(oldRedis) })
	oldLogging := config.IsLogConsumeEnabled()
	config.SetLogConsumeEnabled(false)
	t.Cleanup(func() { config.SetLogConsumeEnabled(oldLogging) })

	for _, scenario := range []struct {
		name      string
		stream    bool
		truncated bool
		writeFail bool
		missing   bool
	}{
		{name: "json"}, {name: "stream", stream: true},
		{name: "truncated_stream", stream: true, truncated: true},
		{name: "failed_json_delivery", writeFail: true},
		{name: "missing_json_receipt", missing: true},
		{name: "missing_stream_receipt", missing: true, stream: true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			start := seedRetryDoubleChargeUser(t)
			c := setupClaudeRetryContext(t, httptest.NewRecorder(), "")
			m := meta.GetByContext(c)
			m.APIType = apitype.AwsClaude
			m.IsStream = scenario.stream
			m.ActualModelName = retryDoubleChargeModel
			body := `{"model":"` + retryDoubleChargeModel + `","max_tokens":1024,"messages":[{"role":"user","content":"Hello"}]}`
			var request ClaudeMessagesRequest
			require.NoError(t, json.Unmarshal([]byte(body), &request))
			var native relaymodel.ClaudeRequest
			require.NoError(t, json.Unmarshal([]byte(body), &native))
			transport := &awsSettlementTransport{stream: scenario.stream,
				body: []byte(`{"id":"msg_settlement","type":"message","role":"assistant","content":[{"type":"text","text":"Hello"}],"usage":` + awsSettlementReceipt + `}`)}
			if scenario.stream {
				transport.body = awsSettlementFrames(t, scenario.truncated)
			}
			if scenario.missing {
				transport.body = nil
			}
			a := &awsadaptor.Adaptor{AwsClient: bedrockruntime.New(bedrockruntime.Options{
				Region: "test-region-1", HTTPClient: &http.Client{Transport: transport}, Retryer: aws.NopRetryer{},
				Credentials: credentials.NewStaticCredentialsProvider("test-access", "test-secret", ""),
			})}
			cfg := a.GetDefaultModelPricing()[retryDoubleChargeModel]
			reservation, admissionErr := preConsumeClaudeMessagesQuota(c, &request, 10, cfg.Ratio, cfg.CompletionRatio, m)
			require.Nil(t, admissionErr)
			require.Positive(t, reservation)
			markPreConsumed(c, reservation)
			_, err := a.ConvertClaudeRequest(c, &native)
			require.NoError(t, err)
			_, err = a.DoRequest(c, m, strings.NewReader(body))
			require.NoError(t, err)
			if scenario.writeFail {
				c.Writer = &awsSettlementFailedWriter{ResponseWriter: c.Writer}
			}
			usage, responseErr := a.DoResponse(c, nil, m)
			require.NotNil(t, usage)
			if scenario.truncated || scenario.writeFail || scenario.missing {
				require.NotNil(t, responseErr)
			} else {
				require.Nil(t, responseErr)
			}
			if scenario.writeFail {
				require.False(t, c.Writer.Written())
			}
			if scenario.missing {
				require.NotEmpty(t, usage.BillingEstimateReason)
			}
			markResponseSettlement(c, usage, responseErr)
			markBillingReconciled(c)
			bctx := detachForBilling(c)
			charged := postConsumeClaudeMessagesQuotaWithTraceID(bctx, "req_aws_settlement_"+scenario.name, "",
				usage, m, &request, cfg.Ratio, reservation, 0, cfg.Ratio, nil, 1, nil, nil)
			drainCriticalTasks(t)
			want := int64(math.Ceil(21*cfg.Ratio + 50*cfg.Ratio*cfg.CompletionRatio +
				1000*cfg.CachedInputRatio + 100*cfg.CacheWrite5mRatio + 200*cfg.CacheWrite1hRatio))
			if scenario.missing {
				want = reservation
			}
			require.Equal(t, want, charged)
			require.Equal(t, charged, start-reloadUserQuota(t))
			var token model.Token
			require.NoError(t, model.DB.Where("id = ?", fallbackTokenID).First(&token).Error)
			require.Equal(t, charged, start-token.RemainQuota)
			require.False(t, BillingAllowsRetry(c), "settled or delivered requests must not be replayed")
			ResetPerAttemptBillingForRetry(bctx, c)
			drainCriticalTasks(t)
			require.Equal(t, charged, start-reloadUserQuota(t), "retry reset must not refund a settled request")
			require.EqualValues(t, 1, transport.calls.Load())
		})
	}
}
