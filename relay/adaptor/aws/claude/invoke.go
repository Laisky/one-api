package aws

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/relay/adaptor/anthropic"
	"github.com/Laisky/one-api/relay/adaptor/aws/utils"
	relaymodel "github.com/Laisky/one-api/relay/model"
)

// invokeError wraps an SDK, protocol, or delivery failure once for the relay.
// It retains real AWS HTTP status codes when available and never logs payloads.
func invokeError(err error) *relaymodel.ErrorWithStatusCode {
	wrapped := utils.WrapErr(errors.Wrap(err, "AWS Claude Invoke"))
	wrapped.StatusCode = http.StatusBadGateway
	var status interface{ HTTPStatusCode() int }
	if errors.As(err, &status) && status.HTTPStatusCode() >= 400 && status.HTTPStatusCode() <= 599 {
		wrapped.StatusCode = status.HTTPStatusCode()
	}
	return wrapped
}

// decodeInvokeJSON decodes a validated JSON value while preserving numeric tool
// arguments. It returns decoding errors instead of rounding numbers to float64.
func decodeInvokeJSON(raw []byte, destination any) error {
	if !json.Valid(raw) {
		return errors.New("invalid Claude response JSON")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(destination); err != nil {
		return errors.Wrap(err, "decode Claude response")
	}
	return nil
}

// Handler executes one non-streaming Claude Invoke request. It returns the last
// verified receipt even when response conversion or client delivery fails, so
// the existing controller can settle consumed tokens rather than refund them.
func Handler(c *gin.Context, client *bedrockruntime.Client, modelName string) (result *relaymodel.ErrorWithStatusCode, usage *relaymodel.Usage) {
	target, err := resolveClaudeInvokeModel(c, client)
	if err != nil {
		return invokeError(err), nil
	}
	body, err := claudeInvokeBody(c)
	if err != nil {
		return invokeError(err), nil
	}
	started := time.Now()
	response, err := client.InvokeModel(gmw.Ctx(c), &bedrockruntime.InvokeModelInput{
		ModelId: aws.String(target), Accept: aws.String("application/json"), ContentType: aws.String("application/json"), Body: body,
	})
	utils.UpdateRegionHealthMetrics(client.Options().Region, err == nil, time.Since(started), err)
	if err != nil {
		return invokeError(err), nil
	}
	// HTTP success proves AWS accepted the invocation, not that zero tokens
	// were billed. Preserve the existing estimated-settlement path when a
	// malformed response prevents receipt recovery, and forbid paid replay.
	defer func() {
		if result != nil && usage == nil {
			usage = &relaymodel.Usage{BillingEstimateReason: "missing_usage_after_accepted_aws_claude_invoke"}
		}
	}()
	if response == nil {
		return invokeError(errors.New("missing Claude Invoke response")), nil
	}
	logger := gmw.GetLogger(c)
	logger.Debug("received AWS Claude Invoke response", zap.Int("body_bytes", len(response.Body)))
	var envelope struct {
		Type  string          `json:"type"`
		Usage json.RawMessage `json:"usage"`
		Error anthropic.Error `json:"error"`
	}
	if err := decodeInvokeJSON(response.Body, &envelope); err != nil {
		return invokeError(err), nil
	}
	var receipt invokeUsageReceipt
	if err := receipt.apply(envelope.Usage); err != nil {
		return invokeError(err), nil
	}
	usage = receipt.snapshot()
	if envelope.Type != "message" || envelope.Error.Type != "" {
		return invokeError(errors.New("Claude Invoke returned an error or invalid message envelope")), usage
	}
	if receipt.input == nil || receipt.output == nil {
		return invokeError(errors.New("Claude Invoke response has no complete token receipt")), usage
	}
	var encoded []byte
	if c.GetBool(ctxkey.ClaudeMessagesNative) {
		var native map[string]json.RawMessage
		if err := json.Unmarshal(response.Body, &native); err != nil {
			return invokeError(errors.Wrap(err, "decode native Claude response")), usage
		}
		name, err := json.Marshal(modelName)
		if err != nil {
			return invokeError(errors.Wrap(err, "encode Claude model name")), usage
		}
		native["model"] = name
		encoded, err = json.Marshal(native)
		if err != nil {
			return invokeError(errors.Wrap(err, "encode native Claude response")), usage
		}
	} else {
		var message anthropic.Response
		if err := decodeInvokeJSON(response.Body, &message); err != nil {
			return invokeError(err), usage
		}
		converted := anthropic.ResponseClaude2OpenAI(c, &message)
		converted.Model, converted.Usage = modelName, *usage
		encoded, err = json.Marshal(converted)
		if err != nil {
			return invokeError(errors.Wrap(err, "encode Claude chat response")), usage
		}
	}
	c.Header("Content-Type", "application/json")
	c.Status(http.StatusOK)
	n, err := c.Writer.Write(encoded)
	if err != nil {
		return invokeError(errors.Wrap(err, "write Claude response")), usage
	}
	if n != len(encoded) {
		return invokeError(errors.WithStack(io.ErrShortWrite)), usage
	}
	return nil, usage
}

// invokeResponseWriter records downstream failures even when an existing
// Responses bridge swallows a writer error. It is local to one request.
type invokeResponseWriter struct {
	gin.ResponseWriter
	err error
}

// Write forwards a byte slice and retains the first failed or short write.
func (w *invokeResponseWriter) Write(p []byte) (int, error) {
	if w.err != nil {
		return 0, w.err
	}
	n, err := w.ResponseWriter.Write(p)
	if err == nil && n != len(p) {
		err = io.ErrShortWrite
	}
	if err != nil {
		w.err = errors.Wrap(err, "write Claude stream")
	}
	return n, w.err
}

// WriteString forwards a string through Write so bridge writes are checked too.
func (w *invokeResponseWriter) WriteString(s string) (int, error) {
	return w.Write([]byte(s))
}
